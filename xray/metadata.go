package xray

import (
	"runtime"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// Name records which X-Ray SDK customer uses.
const Name = "X-Ray YA-SDK for Go"

// ServiceData is the metadata of the user service.
// It is used by all segments that X-Ray YA-SDK sends.
var ServiceData = &schema.Service{
	Runtime:        runtime.Compiler,
	RuntimeVersion: runtime.Version(),
}
