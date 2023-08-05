//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"

	"github.com/shogo82148/aws-xray-yasdk-go/xrayaws-v2/whitelist"
)

type Generator struct {
	buf bytes.Buffer
}

func (g *Generator) Printf(s string, args ...any) {
	fmt.Fprintf(&g.buf, s, args...)
}

func (g *Generator) WriteFile(name string) error {
	src, err := g.Format()
	if err != nil {
		return fmt.Errorf("format: %s: %s:\n\n%s\n", name, err, g.Bytes())
	} else if err := os.WriteFile(name, src, 0644); err != nil {
		return err
	}
	return nil
}

func (g *Generator) Bytes() []byte {
	return g.buf.Bytes()
}

func (g *Generator) Format() ([]byte, error) {
	return format.Source(g.Bytes())
}

func (g *Generator) generateServices(services map[string]*whitelist.Service) {
	keys := make([]string, 0, len(services))
	for key := range services {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	g.Printf("map[string]*whitelist.Service{\n")
	for _, key := range keys {
		g.Printf("%q: ", key)
		g.generateService(services[key])
	}
	g.Printf("},\n")
}

func (g *Generator) generateService(service *whitelist.Service) {
	g.Printf("{\n")
	g.Printf("Operations: \n")
	g.generateOperations(service.Operations)
	g.Printf("},\n")
}

func (g *Generator) generateOperations(operations map[string]*whitelist.Operation) {
	keys := make([]string, 0, len(operations))
	for key := range operations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	g.Printf("map[string]*whitelist.Operation{\n")
	for _, key := range keys {
		g.Printf("%q: ", key)
		g.generateOperation(operations[key])
	}
	g.Printf("},\n")
}

func (g *Generator) generateOperation(operation *whitelist.Operation) {
	g.Printf("{\n")
	if params := operation.RequestDescriptors; params != nil {
		g.Printf("RequestDescriptors: ")
		g.generateDescriptors(params)
	}
	if params := operation.RequestParameters; params != nil {
		g.Printf("RequestParameters: []string{\n")
		for _, param := range params {
			g.Printf("%q,\n", param)
		}
		g.Printf("},\n")
	}
	if params := operation.ResponseDescriptors; params != nil {
		g.Printf("ResponseDescriptors: ")
		g.generateDescriptors(params)
	}
	if params := operation.ResponseParameters; params != nil {
		g.Printf("ResponseParameters: []string{\n")
		for _, param := range params {
			g.Printf("%q,\n", param)
		}
		g.Printf("},\n")
	}
	g.Printf("},\n")
}

func (g *Generator) generateDescriptors(params map[string]*whitelist.Descriptor) {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	g.Printf("map[string]*whitelist.Descriptor{\n")
	for _, key := range keys {
		g.Printf("%q: ", key)
		g.generateDescriptor(params[key])
	}
	g.Printf("},\n")
}

func (g *Generator) generateDescriptor(param *whitelist.Descriptor) {
	g.Printf("{\n")
	if param.Map {
		g.Printf("Map: true,\nGetKeys:%t,\n", param.GetKeys)
	}
	if param.List {
		g.Printf("List: true,\nGetCount:%t,\n", param.GetCount)
	}
	if param.RenameTo != "" {
		g.Printf("RenameTo: %q,\n", param.RenameTo)
	}
	g.Printf("},\n")
}

func generate(path string) {
	list, err := whitelist.ReadFile("AWSWhitelist.json")
	if err != nil {
		log.Fatal(err)
	}

	var g Generator
	g.Printf(`// Code generated by codegen.go; DO NOT EDIT

	package xrayaws

	import "github.com/shogo82148/aws-xray-yasdk-go/xrayaws-v2/whitelist"

	var defaultWhitelist = &whitelist.Whitelist{
		Services: `)
	g.generateServices(list.Services)
	g.Printf("}\n")
	if err := g.WriteFile(path); err != nil {
		log.Fatal(err)
	}
}

func main() {
	generate("default_whitelist.go")
}
