// Package init installs the EC2 plugin at init time.
// To enable this plugin, please import the ec2/init package.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ec2/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ec2"
//	ec2.Init()
package init

import "github.com/shogo82148/aws-xray-yasdk-go/plugins/ec2"

func init() {
	ec2.Init()
}
