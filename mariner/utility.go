package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
)

// this file contains miscellaneous utility functions

// PrintJSON pretty prints a struct as json
func printJSON(i interface{}) {
	var see []byte
	var err error
	see, err = json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v", err)
	}
	fmt.Println(string(see))
}

// GetLastInPath is a utility function. Example i/o:
// in: "#subworkflow_test.cwl/test_expr/file_array"
// out: "file_array"
func lastInPath(s string) (localID string) {
	tmp := strings.Split(s, "/")
	return tmp[len(tmp)-1]
}

func readDir(pwd, dir string) {
	os.Chdir(pwd)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Printf("error reading dir: %v", err)
	}
	fmt.Println("reading ", dir, " from dir ", pwd)
	fmt.Println("found these files:")
	for _, f := range files {
		fmt.Println(f.Name())
	}
}

func struct2String(i interface{}) (s string) {
	j, _ := json.Marshal(i)
	return string(j)
}

// GetRandString returns a random string of length N
func getRandString(n int) string {
	letterBytes := "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
