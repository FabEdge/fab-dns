package apiserver

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
)

const (
	HeaderClusterName = "X-FabEdge-Cluster"
)

type Config struct {
	Address string
	Log     logr.Logger
	Client  client.Client
}

func New(cfg Config) (*http.Server, error) {
	s := &Server{
		Config: cfg,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Get("/api/global-services", s.GetAllGlobalServices)
	r.Post("/api/global-services", s.UploadGlobalService)
	r.Delete("/api/global-services/{namespaceDefault}/{name}", s.deleteEndpoints)

	return &http.Server{
		Addr:    cfg.Address,
		Handler: r,
	}, nil
}

type Server struct {
	Config

	// this lock is used to protect a global service
	// from being changed by requests simultaneously
	// todo: implement object lock
	lock sync.Mutex
}

func (s *Server) GetAllGlobalServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var globalServices apis.GlobalServiceList
	err := s.Client.List(ctx, &globalServices)
	if err != nil {
		s.response(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i, svc := range globalServices.Items {
		// clean useless fields
		svc.ObjectMeta = metav1.ObjectMeta{
			Name:            svc.Name,
			Namespace:       svc.Namespace,
			ResourceVersion: svc.ResourceVersion,
		}
		globalServices.Items[i] = svc
	}

	data, err := json.Marshal(globalServices.Items)
	if err != nil {
		s.response(w, http.StatusInternalServerError, fmt.Sprintf("unable to marshal global services: %s", err))
		s.Log.Error(err, "unable to marshal global services")
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) UploadGlobalService(w http.ResponseWriter, r *http.Request) {
	serviceJson, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.response(w, http.StatusInternalServerError, fmt.Sprintf("failed to read request body: %s", err))
		return
	}

	var gs apis.GlobalService
	if err = json.Unmarshal(serviceJson, &gs); err != nil {
		s.response(w, http.StatusBadRequest, fmt.Sprintf("unabled to unmarshal request body: %s", err))
		return
	}

	if len(gs.Name) == 0 || len(gs.Namespace) == 0 || len(gs.Spec.Ports) == 0 || len(gs.Spec.Endpoints) == 0 {
		s.response(w, http.StatusBadRequest, fmt.Sprintf("data is not valid"))
		return
	}

	if err = s.createOrUpdateGlobalService(gs); err != nil {
		s.response(w, http.StatusInternalServerError, err.Error())
		return
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) createOrUpdateGlobalService(externalService apis.GlobalService) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var (
		localService apis.GlobalService
		key          = client.ObjectKey{Name: externalService.Name, Namespace: externalService.Namespace}
	)

	err := s.Client.Get(ctx, key, &localService)
	if errors.IsNotFound(err) {
		localService = apis.GlobalService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      externalService.Name,
				Namespace: externalService.Namespace,
			},
			Spec: apis.GlobalServiceSpec{
				Type:      externalService.Spec.Type,
				Ports:     externalService.Spec.Ports,
				Endpoints: externalService.Spec.Endpoints,
			},
		}
		return s.Client.Create(ctx, &localService)
	} else if err == nil {
		// remove old endpoints from this cluster
		allEndpoints := removeEndpoints(localService.Spec.Endpoints, externalService.ClusterName)
		allEndpoints = append(allEndpoints, externalService.Spec.Endpoints...)

		// todo: handle cases when ports or type are different
		localService.Spec = apis.GlobalServiceSpec{
			Type:      externalService.Spec.Type,
			Ports:     externalService.Spec.Ports,
			Endpoints: allEndpoints,
		}
		return s.Client.Update(ctx, &localService)
	}

	return err
}

func (s *Server) deleteEndpoints(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "name")
	namespace := chi.URLParam(r, "namespaceDefault")

	s.lock.Lock()
	defer s.lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var (
		svc apis.GlobalService
		key = client.ObjectKey{Name: serviceName, Namespace: namespace}
	)

	err := s.Client.Get(ctx, key, &svc)
	if errors.IsNotFound(err) {
		w.WriteHeader(http.StatusNoContent)
	} else if err != nil {
		s.response(w, http.StatusInternalServerError, fmt.Sprintf("failed to find global service: %s", err))
	} else {
		clusterName := s.getCluster(r)
		svc.Spec.Endpoints = removeEndpoints(svc.Spec.Endpoints, clusterName)

		if len(svc.Spec.Endpoints) == 0 {
			err = s.Client.Delete(ctx, &svc)
		} else {
			err = s.Client.Update(ctx, &svc)
		}

		if err != nil {
			s.response(w, http.StatusInternalServerError, fmt.Sprintf("failed to remove endpoints: %s", err))
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (s *Server) response(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	_, err := w.Write([]byte(msg))
	if err != nil {
		s.Log.Error(err, "failed to write http response")
	}
}

func (s *Server) getCluster(r *http.Request) string {
	return r.Header.Get(HeaderClusterName)
}

func removeEndpoints(endpoints []apis.Endpoint, cluster string) []apis.Endpoint {
	for i := 0; i < len(endpoints); {
		if endpoints[i].Cluster == cluster {
			endpoints = append(endpoints[:i], endpoints[i+1:]...)
			continue
		}
		i++
	}

	return endpoints
}
