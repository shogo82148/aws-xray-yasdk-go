package xray

import (
	"runtime"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// Version records the current X-Ray Go SDK version.
const Version = "0.0.2"

// Name records which X-Ray SDK customer uses.
const Name = "X-Ray YA-SDK for Go"

// ServiceData is the metadata of the user service.
// It is used by all segments that X-Ray YA-SDK sends.
var ServiceData *schema.Service

func init() {
	ServiceData = &schema.Service{
		Compiler:        runtime.Compiler,
		CompilerVersion: runtime.Version(),
	}
}
