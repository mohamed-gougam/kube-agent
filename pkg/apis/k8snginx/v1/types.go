package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TCPServer defines the TCPServer resource.
type TCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TCPServerSpec `json:"spec"`
}

// TCPServerSpec is the spec of the TCPServer resource.
type TCPServerSpec struct {
	ListenPort  int    `json:"listenPort"`
	ServiceName string `json:"serviceName"`
	ServicePort int    `json:"servicePort"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TCPServerList is a list of the TCPServer resources.
type TCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TCPServer `json:"items"`
}
