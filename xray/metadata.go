package xray

import (
	"runtime"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// ServiceData is the metadata for the service.
var ServiceData *schema.Service

func init() {
	ServiceData = &schema.Service{
		Compiler:        runtime.Compiler,
		CompilerVersion: runtime.Version(),
	}
}
