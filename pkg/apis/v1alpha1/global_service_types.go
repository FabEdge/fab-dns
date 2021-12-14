package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceType string

const (
	ClusterIP ServiceType = "ClusterIP"
	Headless  ServiceType = "Headless"
)

// GlobalService is used to represent a service which can be accessed through multi-clusters
// A global services' endpoints can be services if its type is ClusterIP  or pods if its type is Headless
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="The type of global service"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="How long a global service is created"
type GlobalService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GlobalServiceSpec `json:"spec,omitempty"`
}

// GlobalServiceSpec describes global service and the information necessary to consume it.
type GlobalServiceSpec struct {
	// Type represents the type of services which are the backends of a global service
	// Must be ClusterIP or Headless
	// +kubebuilder:validation:Enum=ClusterIP;Headless
	Type ServiceType `json:"type,omitempty"`

	Ports []ServicePort `json:"ports,omitempty"`

	Endpoints []discoveryv1.Endpoint `json:"endpoints,omitempty"`
}

// Endpoint represents a single logical "backend" implementing a service.
type Endpoint struct {
	// addresses of this endpoint. The contents of this field are interpreted
	// according to the corresponding EndpointSlice addressType field. Consumers
	// must handle different types of addresses in the context of their own
	// capabilities. This must contain at least one address but no more than
	// 100.
	// +listType=set
	Addresses []string `json:"addresses"`
	// hostname of this endpoint. This field may be used by consumers of
	// endpoints to distinguish endpoints from each other (e.g. in DNS names).
	// Multiple endpoints which use the same hostname should be considered
	// fungible (e.g. multiple A values in DNS). Must be lowercase and pass DNS
	// Label (RFC 1123) validation.
	// +optional
	Hostname *string `json:"hostname,omitempty"`
	// targetRef is a reference to a Kubernetes object that represents this
	// endpoint.
	// +optional
	TargetRef *corev1.ObjectReference `json:"targetRef,omitempty"`

	// Zone indicates the zone where the endpoint is located
	Zone string `json:"zone,omitempty"`
	// Region indicates the region where the endpoint is located
	Region string `json:"region,omitempty"`
}

// ServicePort represents the port on which the service is exposed
type ServicePort struct {
	// The name of this port within the service. This must be a DNS_LABEL.
	// All ports within a ServiceSpec must have unique names. When considering
	// the endpoints for a Service, this must match the 'name' field in the
	// EndpointPort.
	// Optional if only one ServicePort is defined on this service.
	// +optional
	Name string `json:"name,omitempty"`

	// The IP protocol for this port. Supports "TCP", "UDP", and "SCTP".
	// Default is TCP.
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`

	// The application protocol for this port.
	// This field follows standard Kubernetes label syntax.
	// Un-prefixed names are reserved for IANA standard service names (as per
	// RFC-6335 and http://www.iana.org/assignments/service-names).
	// Non-standard protocols should use prefixed names such as
	// mycompany.com/my-custom-protocol.
	// Field can be enabled with ServiceAppProtocol feature gate.
	// +optional
	AppProtocol *string `json:"appProtocol,omitempty"`

	// The port that will be exposed by this service.
	Port int32 `json:"port,omitempty"`
}

// GlobalServiceList contains a list of global services
// +kubebuilder:object:root=true
type GlobalServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalService `json:"items,omitempty"`
}
