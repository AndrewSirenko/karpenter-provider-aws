/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/samber/lo"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"

	"github.com/aws/karpenter-provider-aws/pkg/operator/options"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s path/to/markdown.md", os.Args[0])
	}
	outputFileName := os.Args[1]
	mdFile, err := os.ReadFile(outputFileName)
	if err != nil {
		log.Printf("Can't read %s file: %v", os.Args[1], err)
		os.Exit(2)
	}

	fs := &coreoptions.FlagSet{
		FlagSet: flag.NewFlagSet("karpenter", flag.ContinueOnError),
	}
	(&coreoptions.Options{}).AddFlags(fs)
	(&options.Options{}).AddFlags(fs)

	envVarsBlock := "| Environment Variable | CLI Flag | Description |\n"
	envVarsBlock += "|--|--|--|\n"
	fs.VisitAll(func(f *flag.Flag) {
		if f.DefValue == "" {
			envVarsBlock += fmt.Sprintf("| %s | %s | %s|\n", strings.ReplaceAll(strings.ToUpper(f.Name), "-", "_"), "\\-\\-"+f.Name, f.Usage)
		} else {
			envVarsBlock += fmt.Sprintf("| %s | %s | %s (default = %s)|\n", strings.ReplaceAll(strings.ToUpper(f.Name), "-", "_"), "\\-\\-"+f.Name, f.Usage, f.DefValue)
		}
	})

	highCardinalityBlock := "| Name | Stage | Metrics | When disabled | Cardinality |\n"
	highCardinalityBlock += "|--|--|--|--|--|\n"
	for _, m := range coreoptions.KarpenterHighCardinalityMetrics {
		highCardinalityBlock += fmt.Sprintf("| %s | %s | %s | %s | %s |\n", m.Name, lo.Capitalize(string(m.Stage)),
			strings.Join(lo.Map(m.Metrics, func(name string, _ int) string { return "`" + name + "`" }), "<br>"), m.Help, m.Cardinality)
	}

	doc := replaceGenerated(string(mdFile), "hack/docs/configuration_gen/main.go", envVarsBlock)
	doc = replaceGenerated(doc, "hack/docs/configuration_gen/main.go high cardinality metrics", highCardinalityBlock)

	log.Println("writing output to", outputFileName)
	f, err := os.Create(outputFileName)
	if err != nil {
		log.Fatalf("unable to open %s to write generated output: %v", outputFileName, err)
	}
	f.WriteString(doc)
}

// replaceGenerated replaces the content between the generated comment markers named by source.
func replaceGenerated(doc, source, content string) string {
	genStart := fmt.Sprintf("[comment]: <> (the content below is generated from %s)", source)
	genEnd := fmt.Sprintf("[comment]: <> (end docs generated content from %s)", source)
	startDocSections := strings.Split(doc, genStart)
	if len(startDocSections) != 2 {
		log.Fatalf("expected one generated comment block start for %s but got %d", source, len(startDocSections)-1)
	}
	endDocSections := strings.Split(doc, genEnd)
	if len(endDocSections) != 2 {
		log.Fatalf("expected one generated comment block end for %s but got %d", source, len(endDocSections)-1)
	}
	return fmt.Sprintf("%s%s\n\n%s\n%s%s", startDocSections[0], genStart, content, genEnd, endDocSections[1])
}
