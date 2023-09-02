// Package beanstalk provides plugin for AWS Elastic Beanstalk.
// The plugin collects the information of Elastic Beanstalk environment,
// and record them.
//
// To enable this plugin, please import the beanstalk/init package.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/beanstalk/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/beanstalk"
//	beanstalk.Init()
package beanstalk

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

const configPath = "/var/elasticbeanstalk/xray/environment.conf"

type plugin struct {
	ElasticBeanstalk *schema.ElasticBeanstalk
}

var once sync.Once

// Init activates Beanstalk Plugin at runtime.
func Init() {
	once.Do(initBeanstalkPlugin)
}

func initBeanstalkPlugin() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if runtime.GOOS != "linux" {
		return
	}

	f, err := os.Open(configPath)
	if err != nil {
		// it seems not to be in Elastic Beanstalk environment.
		// just ignore error.
		xraylog.Debugf(ctx, "failed to read %q: %v", configPath, err)
		return
	}
	defer f.Close()

	var conf schema.ElasticBeanstalk
	dec := json.NewDecoder(f)
	if err := dec.Decode(&conf); err != nil {
		xraylog.Debugf(ctx, "failed to decode: %v", err)
		return
	}
	xray.AddPlugin(&plugin{
		ElasticBeanstalk: &conf,
	})
}

// HandleSegment implements Plugin.
func (p *plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetElasticBeanstalk(p.ElasticBeanstalk)
}

// Origin implements Plugin.
func (*plugin) Origin() string { return schema.OriginElasticBeanstalk }
