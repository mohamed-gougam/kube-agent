package validation

import (
	v1 "github.com/mohamed-gougam/kube-agent/pkg/apis/k8snginx/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateTCPServer returns error if tcpServer is not a valid TCPServer.
func ValidateTCPServer(tcpServer *v1.TCPServer) error {
	errs := validateTCPServerSpec(&tcpServer.Spec, field.NewPath("spec"))
	return errs.ToAggregate()
}

func validateTCPServerSpec(tcpServerSpec *v1.TCPServerSpec, fieldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}

	errs = append(errs, validatePort(tcpServerSpec.ListenPort, fieldPath.Child("listenPort"))...)
	errs = append(errs, validateServiceName(tcpServerSpec.ServiceName, fieldPath.Child("serviceName"))...)
	errs = append(errs, validatePort(tcpServerSpec.ServicePort, fieldPath.Child("servicePort"))...)

	return errs
}

func validatePort(port int, fieldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}

	// TCP Port 37 returns date_&_time_now and is set as equivalent of 404 not found of http
	// it's set by default if TCPServer has service that doesn't exist yet
	if port != 37 {
		for _, msg := range validation.IsValidPortNum(port) {
			errs = append(errs, field.Invalid(fieldPath, port, msg))
		}
		return errs
	}

	errs = append(errs, field.Invalid(fieldPath, port, "Port 37 is reserved for Time."))

	return errs
}

func validateServiceName(name string, fieldPath *field.Path) field.ErrorList {
	return validateDNS1035Label(name, fieldPath)
}

func validateDNS1035Label(name string, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if name == "" {
		return append(allErrs, field.Required(fieldPath, ""))
	}

	for _, msg := range validation.IsDNS1035Label(name) {
		allErrs = append(allErrs, field.Invalid(fieldPath, name, msg))
	}

	return allErrs
}
