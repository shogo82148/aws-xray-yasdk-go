// Package init installs the EKS plugin at init time.
// To enable this plugin, please import the eks/init package.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/eks/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/eks"
//	eks.Init()
package init

import "github.com/shogo82148/aws-xray-yasdk-go/plugins/eks"

func init() {
	eks.Init()
}
