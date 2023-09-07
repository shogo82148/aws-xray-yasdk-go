// Package init installs the Beanstalk plugin at init time.
// To enable this plugin, please import the beanstalk/init package.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/beanstalk/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/beanstalk"
//	beanstalk.Init()
package init

import "github.com/shogo82148/aws-xray-yasdk-go/plugins/beanstalk"

func init() {
	beanstalk.Init()
}
