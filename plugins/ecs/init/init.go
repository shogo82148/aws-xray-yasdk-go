// Package init installs the ECS plugin at init time.
// To enable this plugin, please import the ecs/init package.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ecs/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ecs"
//	ecs.Init()
package init

import "github.com/shogo82148/aws-xray-yasdk-go/plugins/ecs"

func init() {
	ecs.Init()
}
