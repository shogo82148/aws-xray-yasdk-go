package xray

import (
	"reflect"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// Plugin is the interface of AWS X-Ray plugin.
type Plugin interface {
	// HandleSegment is called by AWS X-Ray YA-SDK
	// before submitting the root segment.
	// The document is the raw data of the segment, and plugins can rewrite it.
	HandleSegment(segment *Segment, document *schema.Segment)

	// Origin returns the type of AWS resource that the plugin detected.
	// If the plugin can't detect any type, it returns empty string.
	Origin() string
}

var muPlugins sync.RWMutex
var plugins []Plugin

// AddPlugin adds a plugin.
func AddPlugin(plugin Plugin) {
	if plugin == nil {
		panic("xray: plugin should not be nil")
	}
	muPlugins.Lock()
	defer muPlugins.Unlock()
	plugins = append(plugins, plugin)
}

func getPlugins() []Plugin {
	muPlugins.RLock()
	defer muPlugins.RUnlock()
	return plugins
}

var _ Plugin = (*xrayPlugin)(nil)

// xrayPlugin injects information about X-Ray YA-SDK.
type xrayPlugin struct {
	sdkVersion string
}

func init() {
	AddPlugin(&xrayPlugin{
		sdkVersion: getVersion(),
	})
}

// HandleSegment implements Plugin.
func (p *xrayPlugin) HandleSegment(seg *Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetXRay(&schema.XRay{
		SDKVersion:       p.sdkVersion,
		SDK:              Name,
		SamplingRuleName: seg.ruleName,
	})
}

// Origin implements Plugin.
func (xrayPlugin) Origin() string { return "" }

func getVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}

	// get the package path of the sdk
	typ := reflect.TypeOf(xrayPlugin{})
	pkg := typ.PkgPath()

	version := Version
	depth := 0
	for _, dep := range info.Deps {
		// search the most specific module
		if strings.HasPrefix(pkg, dep.Path) && len(dep.Path) > depth {
			version = dep.Version
		}
	}
	return version
}
