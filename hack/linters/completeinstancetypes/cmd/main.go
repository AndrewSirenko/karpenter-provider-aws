package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/aws/karpenter-provider-aws/hack/linters/completeinstancetypes"
)

func main() { singlechecker.Main(completeinstancetypes.Analyzer) }
