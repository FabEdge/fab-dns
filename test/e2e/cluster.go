package e2e

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/test/e2e/framework"
)

type Cluster struct {
	name                        string
	zone                        string
	region                      string
	role                        string
	config                      *rest.Config
	client                      client.Client
	clientset                   kubernetes.Interface
	podsReady                   bool
	clusterIPGlobalServiceReady bool
	headlessGlobalServiceReady  bool
}

func (c Cluster) ready() bool {
	framework.Logf("cluster %s status podsReady=%t clusterIPGlobalServiceReady=%t headlessGlobalServiceReady=%t",
		c.name, c.podsReady, c.clusterIPGlobalServiceReady, c.headlessGlobalServiceReady)
	return c.podsReady && c.clusterIPGlobalServiceReady && c.headlessGlobalServiceReady
}

func generateCluster(cfgDir, ip string) (cluster Cluster, err error) {
	// path e.g. /tmp/e2ekubeconfig/10.20.8.20
	cfg, err := clientcmd.BuildConfigFromFlags("", path.Join(cfgDir, ip))
	if err != nil {
		return
	}

	// rewrite config host, e.g. "https://vip.edge.io:6443" => "https://10.20.8.20:6443"
	segments := strings.Split(cfg.Host, ":")
	if len(segments) < 2 {
		return cluster, fmt.Errorf("cluster ip <%s> kubeconfig server %s can not rewrite to ip:port style", ip, cfg.Host)
	}
	segments[1] = fmt.Sprintf("//%s", ip)
	cfg.Host = strings.Join(segments, ":")

	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		return
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return
	}

	cluster = Cluster{
		config:    cfg,
		client:    cli,
		clientset: clientset,
	}

	// get cluster name,role,zone,region
	err = cluster.generateDetail()
	if err != nil {
		return
	}
	return cluster, nil
}

func (c *Cluster) generateDetail() error {
	// deploy fabedge-operator, ns fabedge
	var err error
	args := []string{"--cluster=", "--cluster-role="}
	args, err = c.getDeployArgs("fabedge-operator", "fabedge", args...)
	if err != nil {
		return err
	}
	c.name = args[0][len("--cluster="):]
	c.role = args[1][len("--cluster-role="):]

	if len(c.name) == 0 || len(c.role) == 0 {
		return fmt.Errorf("clusterName=%s or clusterRole=%s", c.name, c.role)
	}

	// deploy service-hub, ns fabedge
	args = []string{"--cluster=", "--zone=", "--region="}
	args, err = c.getDeployArgs("service-hub", "fabedge", args...)
	if err != nil {
		return err
	}
	clusterName := args[0][len("--cluster="):]
	if clusterName != c.name {
		return fmt.Errorf("service-hub set different cluster name %s", clusterName)
	}
	c.zone = args[1][len("--zone="):]
	c.region = args[2][len("--region="):]

	return nil
}

func (c Cluster) prepareNamespace(namespace string) {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	_ = c.client.Delete(context.Background(), &ns)

	// 等待上次的测试资源清除
	err := framework.WaitForNamespacesDeleted(c.client, []string{namespace}, 5*time.Minute)
	if err != nil {
		framework.Failf("cluster %s namespace %q is not deleted. err: %v", c.name, namespace, err)
	}

	framework.Logf("cluster %s create new test namespace: %s", c.name, namespace)
	createObject(c.client, &ns)
}

func (c Cluster) prepareStatefulSet(name, namespace string, replicas int32) {
	statefulSet := v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKeyInstance: serviceCloudHeadless,
				},
			},
			Replicas:    &replicas,
			ServiceName: serviceCloudHeadless,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKeyApp:      appNetTool,
						labelKeyInstance: serviceCloudHeadless,
					},
				},
				Spec: podSpecWithAffinity(),
			},
		},
	}
	createObject(c.client, &statefulSet)
}

func (c Cluster) prepareDeployment(name, namespace string, replicas int32) {
	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKeyInstance: serviceCloudClusterIP,
				},
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKeyApp:      appNetTool,
						labelKeyInstance: serviceCloudClusterIP,
					},
				},
				Spec: podSpecWithAffinity(),
			},
		},
	}
	createObject(c.client, &deployment)
}

func (c Cluster) prepareDebugPod(name, namespace string) {
	debugPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				labelKeyApp: appNetTool,
			},
		},
		Spec: podSpecWithAffinity(),
	}
	createObject(c.client, &debugPod)
}

func (c Cluster) prepareService(name, namespace string, isHeadless bool) {
	framework.Logf("create and export service %s/%s on %s", namespace, name, c.name)
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				// export local cluster service
				labelKeyGlobalService: "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				labelKeyInstance: name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "default",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if isHeadless {
		svc.Spec.ClusterIP = corev1.ClusterIPNone
	}

	createObject(c.client, &svc)
}

func (c *Cluster) waitForClusterPodsReady(wg *sync.WaitGroup, namespace string) {
	defer wg.Done()

	framework.Logf("Waiting for cluster %s all pods to be ready", c.name)
	timeout := time.Duration(framework.TestContext.WaitTimeout) * time.Second
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		var pods corev1.PodList
		err := c.client.List(context.TODO(), &pods, client.InNamespace(namespace), client.MatchingLabels{
			labelKeyApp: appNetTool,
		})
		if err != nil {
			return false, err
		}

		if len(pods.Items) == 0 {
			return false, nil
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return false, nil
			}
		}

		// wait the pods to be ready, not only to be running, especially on slow environment
		time.Sleep(15 * time.Second)

		c.podsReady = true
		return true, nil
	})

	if err != nil {
		framework.Logf("net-tool pods in cluster %s are not ready after %d seconds. Error: %v", c.name, framework.TestContext.WaitTimeout, err)
	}
}

func (c Cluster) generateInClusterGlobalServiceEndpoints(name, namespace string, isHeadless bool) (endpoints []apis.Endpoint, err error) {
	if isHeadless {
		var headlessEps corev1.Endpoints
		err = c.client.Get(context.TODO(),
			client.ObjectKey{Name: name, Namespace: namespace},
			&headlessEps)
		if err != nil {
			return
		}

		for _, subset := range headlessEps.Subsets {
			for _, address := range subset.Addresses {
				hostname := address.Hostname
				ep := apis.Endpoint{
					Hostname:  &hostname,
					Addresses: []string{address.IP},
					Cluster:   c.name,
					Zone:      c.zone,
					Region:    c.region,
				}
				endpoints = append(endpoints, ep)
			}
		}
		return
	}

	clusterip, err := c.getServiceIP(name, namespace)
	if err != nil {
		return nil, err
	}
	endpoints = append(endpoints, apis.Endpoint{
		Addresses: []string{clusterip},
		Cluster:   c.name,
		Zone:      c.zone,
		Region:    c.region,
	})
	return
}

func (c *Cluster) waitForGlobalServicesReady(wg *sync.WaitGroup, namespace string, expectedGlobalServices map[string]apis.GlobalService) {
	defer wg.Done()

	framework.Logf("Waiting for cluster %s all global services to be ready", c.name)
	timeout := time.Duration(framework.TestContext.WaitTimeout) * time.Second
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		if !c.clusterIPGlobalServiceReady {
			ready, err := c.checkGlobalServiceReady(serviceCloudClusterIP, namespace, apis.ClusterIP, expectedGlobalServices[string(apis.ClusterIP)])
			if err != nil {
				return false, err
			}
			if !ready {
				return false, nil
			}
			c.clusterIPGlobalServiceReady = true
		}

		if !c.headlessGlobalServiceReady {
			ready, err := c.checkGlobalServiceReady(serviceCloudHeadless, namespace, apis.Headless, expectedGlobalServices[string(apis.Headless)])
			if err != nil {
				return false, err
			}
			if !ready {
				return false, nil
			}
			c.headlessGlobalServiceReady = true
		}

		return true, nil
	})

	if err != nil {
		framework.Logf("globalservices in cluster %s are not ready after %d seconds. Error: %v", c.name, framework.TestContext.WaitTimeout, err)
	}
}

func (c Cluster) checkGlobalServiceReady(name, namespace string, serviceType apis.ServiceType, expectedGlobalService apis.GlobalService) (bool, error) {
	var globalservice apis.GlobalService
	err := c.client.Get(context.TODO(),
		client.ObjectKey{Name: name, Namespace: namespace},
		&globalservice)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if globalservice.Spec.Type != serviceType {
		return false, fmt.Errorf("get globalservice %s type is %s, %s type is expected",
			name, string(globalservice.Spec.Type), string(serviceType))
	}

	if len(globalservice.Spec.Endpoints) == 0 ||
		len(globalservice.Spec.Endpoints) != len(expectedGlobalService.Spec.Endpoints) {
		return false, nil
	}

	isHeadless := serviceType == apis.Headless
	for _, expect := range expectedGlobalService.Spec.Endpoints {
		contains := false
		for _, ep := range globalservice.Spec.Endpoints {
			if equal(expect, ep, isHeadless) {
				contains = true
				break
			}
		}

		if !contains {
			return false, nil
		}
	}
	return true, nil
}

func (c Cluster) getServiceIP(servicename, namespace string) (string, error) {
	svc, err := c.clientset.CoreV1().Services(namespace).Get(context.TODO(), servicename, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return svc.Spec.ClusterIP, nil
}

func (c Cluster) getDeployArgs(name, namespace string, argKeys ...string) ([]string, error) {
	// e.g. deploy service-hub, ns fabedge
	dp, err := c.clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	args := dp.Spec.Template.Spec.Containers[0].Args
	results := make([]string, len(argKeys))
	indexes := make(map[string]int, len(argKeys))
	for i, key := range argKeys {
		indexes[key] = i
	}
	for _, v := range args {
		for _, key := range argKeys {
			if strings.HasPrefix(v, key) {
				results[indexes[key]] = v
			}
		}
	}

	return results, nil
}

func (c Cluster) execCurl(pod corev1.Pod, url string) (string, string, error) {
	timeout := fmt.Sprint(framework.TestContext.CurlTimeout)
	return c.execute(pod, []string{"curl", "-sS", "-m", timeout, url})
}

func (c Cluster) execute(pod corev1.Pod, cmd []string) (string, string, error) {
	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: pod.Spec.Containers[0].Name,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil && framework.TestContext.ShowExecError {
		framework.Logf("failed to execute cmd: %s. stderr: %s. err: %s", strings.Join(cmd, " "), stderr.String(), err)
	}

	return stdout.String(), stderr.String(), err
}
