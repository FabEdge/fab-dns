package apiserver

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/service-hub/types"
)

const (
	HeaderClusterName = "X-FabEdge-Cluster"

	PathHeartbeat      = "/api/heartbeat"
	PathGlobalServices = "/api/global-services"
)

type Config struct {
	Address               string
	Log                   logr.Logger
	Client                client.Client
	ClusterStore          *types.ClusterStore
	GlobalServiceManager  types.GlobalServiceManager
	ClusterExpireDuration time.Duration
	RequestTimeout        time.Duration
}

func New(cfg Config) (*http.Server, error) {
	s := &Server{
		Config: cfg,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer, s.updateClusterExpireTime)
	r.Get(PathHeartbeat, s.Heartbeat)
	r.Get(PathGlobalServices, s.GetAllGlobalServices)
	r.Post(PathGlobalServices, s.UploadGlobalService)
	r.Delete(PathGlobalServices+"/{namespaceDefault}/{name}", s.deleteEndpoints)

	return &http.Server{
		Addr:    cfg.Address,
		Handler: r,
	}, nil
}

type Server struct {
	Config
}

func (s *Server) Heartbeat(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetAllGlobalServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), s.RequestTimeout)
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

	defer func() {
		cluster := s.ClusterStore.New(s.getClusterName(r))
		cluster.AddServiceKey(client.ObjectKey{
			Name:      gs.Name,
			Namespace: gs.Namespace,
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), s.RequestTimeout)
	defer cancel()

	if err = s.GlobalServiceManager.CreateOrMergeGlobalService(ctx, gs); err != nil {
		s.response(w, http.StatusInternalServerError, err.Error())
		return
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) deleteEndpoints(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "name")
	namespace := chi.URLParam(r, "namespaceDefault")
	clusterName := s.getClusterName(r)

	ctx, cancel := context.WithTimeout(context.Background(), s.RequestTimeout)
	defer cancel()

	err := s.GlobalServiceManager.RevokeGlobalService(ctx, clusterName, namespace, serviceName)
	if err != nil {
		s.response(w, http.StatusInternalServerError, fmt.Sprintf("failed to find global service: %s", err))
	} else {
		cluster := s.ClusterStore.Get(clusterName)
		if cluster != nil {
			cluster.RemoveServiceKey(client.ObjectKey{
				Name:      serviceName,
				Namespace: namespace,
			})
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) response(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	_, err := w.Write([]byte(msg))
	if err != nil {
		s.Log.Error(err, "failed to write http response")
	}
}

func (s *Server) updateClusterExpireTime(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		clusterName := s.getClusterName(r)
		if clusterName == "" {
			next.ServeHTTP(w, r)
			return
		}

		cluster := s.ClusterStore.New(clusterName)
		cluster.SetExpireTime(time.Now().Add(s.ClusterExpireDuration))

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func (s *Server) getClusterName(r *http.Request) string {
	return r.Header.Get(HeaderClusterName)
}
