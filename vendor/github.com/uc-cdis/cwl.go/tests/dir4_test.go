package cwlgotest

import (
	"sort"
	"testing"

	cwl "github.com/otiai10/cwl.go"
	. "github.com/otiai10/mint"
)

func TestDecode_dir4(t *testing.T) {
	f := load("dir4.cwl")
	root := cwl.NewCWL()
	err := root.Decode(f)
	Expect(t, err).ToBe(nil)

	Expect(t, root.Requirements[0].Class).ToBe("ShellCommandRequirement")
	Expect(t, root.Inputs[0].ID).ToBe("inf")
	Expect(t, root.Inputs[0].Types[0].Type).ToBe("File")
	Expect(t, root.Outputs[0].ID).ToBe("outlist")
	Expect(t, root.Outputs[0].Types[0].Type).ToBe("File")
	Expect(t, root.Outputs[0].Binding.Glob).ToBe([]string{"output.txt"})
	sort.Sort(root.Arguments)
	Expect(t, root.Arguments[0].Value).ToBe("cd")
	Expect(t, root.Arguments[1].Value).ToBe("$(inputs.inf.dirname)/xtestdir")
	Expect(t, root.Arguments[2].Binding.ShellQuote).ToBe(false)
	Expect(t, root.Arguments[2].Binding.ValueFrom.Key()).ToBe("&&")
	Expect(t, root.Arguments[3].Value).ToBe("find")
	Expect(t, root.Arguments[4].Value).ToBe(".")
	Expect(t, root.Arguments[5].Binding.ShellQuote).ToBe(false)
	Expect(t, root.Arguments[5].Binding.ValueFrom.Key()).ToBe("|")
	Expect(t, root.Arguments[6].Value).ToBe("sort")
	Expect(t, root.Stdout).ToBe("output.txt")
}