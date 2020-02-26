package conformance

import (
	"fmt"
	"testing"
)

// incomplete
func NotTestFilter(t *testing.T) {
	suite, err := loadConfig(config)
	if err != nil {
		t.Errorf("failed to load tests")
	}

	// trueVal := true
	filters := &FilterSet{
		// ShouldFail: &trueVal,
		Tags:  []string{},
		Label: "",
		ID:    []int{},
	}

	fmt.Println("original length: ", len(suite))

	// apply filter to test list
	filtered := filters.apply(suite)

	fmt.Println("filtered length: ", len(filtered))

	fmt.Println("filters:")
	printJSON(filters)

	fmt.Println("filtered results:")
	printJSON(filtered)
}

func TestInputsCollector(t *testing.T) {
	suite, err := loadConfig(config)
	if err != nil {
		t.Errorf("failed to load tests")
	}
	// fixme: assignment to nil error
	inputs, err := InputFiles(suite)

	fmt.Println("inputs:")
	printJSON(inputs)

	if err != nil {
		t.Errorf("collect routine failed: %v", err)
	}
}
