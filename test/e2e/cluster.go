package e2e

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
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
	"github.com/fabedge/fab-dns/pkg/service-hub/exporter"
	"github.com/fabedge/fab-dns/test/e2e/framework"
)

type Cluster struct {
	name          string
	zone          string
	region        string
	config        *rest.Config
	client        client.Client
	clientset     kubernetes.Interface
	podsReady     bool
	servicesReady bool
}

func (c Cluster) ready() bool {
	return c.podsReady && c.servicesReady
}

func generateCluster(kubeconfigPath string) (cluster Cluster, err error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return
	}

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

	// get cluster name,zone,region
	args := []string{"--cluster=", "--zone=", "--region="}
	args, err = cluster.getDeployArguments("service-hub", "fabedge", args...)
	if err != nil {
		return
	}
	cluster.name = args[0]
	cluster.zone = args[1]
	cluster.region = args[2]
	return cluster, nil
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

func (c Cluster) prepareMySQLStatefulSet(namespace string) {
	var (
		name     = nameMySQL
		replicas = int32(2)
	)

	statefulSet := v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKeyApp: name,
				},
			},
			Replicas:    &replicas,
			ServiceName: serviceNameMySQL,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKeyApp: name,
					},
				},
				Spec: podSpecWithAffinity(),
			},
		},
	}
	createObject(c.client, &statefulSet)
}

func (c Cluster) prepareNginxDeployment(namespace string) {
	var (
		name     = nameNginx
		replicas = int32(2)
	)
	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKeyApp: name,
				},
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKeyApp: name,
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
				labelKeyApp: nameNetTool,
			},
		},
		Spec: podSpecWithAffinity(),
	}
	createObject(c.client, &debugPod)
}

func (c Cluster) prepareService(name, namespace, appName string, isHeadless bool, ipFamilies []corev1.IPFamily) {
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
				labelKeyApp: appName,
			},
			IPFamilies: ipFamilies,
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
			labelKeyApp: nameNetTool,
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
		time.Sleep(5 * time.Second)

		c.podsReady = true
		return true, nil
	})

	if err != nil {
		framework.Logf("net-tool pods in cluster %s are not ready after %d seconds. Error: %v", c.name, framework.TestContext.WaitTimeout, err)
	}
}

func (c Cluster) generateGlobalServiceEndpoints(name, namespace string, serviceType apis.ServiceType) (endpoints []apis.Endpoint, err error) {
	if serviceType == apis.Headless {
		return exporter.GetEndpointsOfHeadlessService(c.client, context.Background(), namespace, name, exporter.ClusterInfo{
			Name:   c.name,
			Zone:   c.zone,
			Region: c.region,
		})
	}

	clusterIPs, err := c.getServiceIP(name, namespace)
	if err != nil {
		return nil, err
	}
	endpoints = append(endpoints, apis.Endpoint{
		Addresses: clusterIPs,
		Cluster:   c.name,
		Zone:      c.zone,
		Region:    c.region,
	})
	return
}

//func (c Cluster) getEndpointsOfHeadlessService(namespace, serviceName string) ([]apis.Endpoint, error) {
//	var endpointSliceList discoveryv1.EndpointSliceList
//	err := c.client.List(context.Background(), &endpointSliceList,
//		client.InNamespace(namespace),
//		client.MatchingLabels{
//			"kubernetes.io/service-name": serviceName,
//		},
//	)
//	if err != nil {
//		return nil, err
//	}
//
//	var endpoints []apis.Endpoint
//	endpointByName := make(map[string]*apis.Endpoint)
//	for _, es := range endpointSliceList.Items {
//		for _, ep := range es.Endpoints {
//			if ep.TargetRef == nil {
//				continue
//			}
//
//			exportedEndpoint := apis.Endpoint{
//				Cluster:   c.name,
//				Zone:      c.zone,
//				Region:    c.region,
//				Addresses: ep.Addresses,
//				Hostname:  ep.Hostname,
//				TargetRef: ep.TargetRef,
//			}
//
//			if e, found := endpointByName[ep.TargetRef.Name]; found {
//				e.Addresses = append(e.Addresses, ep.Addresses...)
//			} else {
//				endpoints = append(endpoints, exportedEndpoint)
//				endpointByName[ep.TargetRef.Name] = &exportedEndpoint
//			}
//		}
//	}
//
//	return endpoints, nil
//}

func (c *Cluster) waitForGlobalServicesReady(wg *sync.WaitGroup, namespace string, expectedGlobalServices []apis.GlobalService) {
	defer wg.Done()

	framework.Logf("Waiting for cluster %s all global services to be ready", c.name)
	timeout := time.Duration(framework.TestContext.WaitTimeout) * time.Second

	readyServiceNames := make(map[string]bool)
	allServicesReady := false
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		for _, gs := range expectedGlobalServices {
			if readyServiceNames[gs.Name] {
				continue
			}

			ready, err := c.checkGlobalServiceReady(gs.Name, namespace, gs)
			if err != nil {
				return false, err
			}

			readyServiceNames[gs.Name] = ready
		}

		allServicesReady = false
		for _, b := range readyServiceNames {
			allServicesReady = allServicesReady || b
		}

		return allServicesReady, nil
	})

	if err != nil {
		framework.Logf("global services in cluster %s are not ready after %d seconds. Error: %v", c.name, framework.TestContext.WaitTimeout, err)
	}

	c.servicesReady = allServicesReady
}

func (c Cluster) checkGlobalServiceReady(name, namespace string, expectedGlobalService apis.GlobalService) (bool, error) {
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

	if globalservice.Spec.Type != expectedGlobalService.Spec.Type {
		return false, fmt.Errorf("%s globalservice type is %s, but expected type is %s",
			name, string(globalservice.Spec.Type), string(expectedGlobalService.Spec.Type))
	}

	if len(globalservice.Spec.Endpoints) != len(expectedGlobalService.Spec.Endpoints) {
		return false, nil
	}

	for _, expect := range expectedGlobalService.Spec.Endpoints {
		contains := false
		for _, ep := range globalservice.Spec.Endpoints {
			if equalEndpoints(expect, ep) {
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

func (c Cluster) getServiceIP(servicename, namespace string) ([]string, error) {
	svc, err := c.clientset.CoreV1().Services(namespace).Get(context.TODO(), servicename, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return svc.Spec.ClusterIPs, nil
}

func (c Cluster) getDeployArguments(name, namespace string, argKeys ...string) ([]string, error) {
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
				results[indexes[key]] = v[len(key):]
			}
		}
	}

	return results, nil
}

func (c Cluster) execCurl(pod corev1.Pod, url string) (string, string, error) {
	timeout := fmt.Sprint(framework.TestContext.CurlTimeout)
	return c.execute(pod, []string{"curl", "-sS", "-m", timeout, url})
}

func (c Cluster) execCurl6(pod corev1.Pod, url string) (string, string, error) {
	timeout := fmt.Sprint(framework.TestContext.CurlTimeout)
	return c.execute(pod, []string{"curl", "-6", "-sS", "-m", timeout, url})
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

func equalEndpoints(a, b apis.Endpoint) bool {
	if a.Hostname != nil && b.Hostname != nil {
		if *(a.Hostname) != *(b.Hostname) {
			return false
		}
	} else if a.Hostname != b.Hostname {
		return false
	}
	switch {
	case a.Cluster != b.Cluster:
		return false
	case a.Zone != b.Zone:
		return false
	case a.Region != b.Region:
		return false
	case !reflect.DeepEqual(a.Addresses, b.Addresses):
		return false
	}
	return true
}
