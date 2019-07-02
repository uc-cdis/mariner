package mariner

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// this file contains code for handling/processing file objects

// File represents a CWL file object
// NOTE: the json representation of field names is what gets loaded into js vm
// ----- see PreProcessContext() and accompanying note of explanation.
// ----- these json aliases are the fieldnames defined by cwl for cwl File objects
//
// see: see: https://www.commonwl.org/v1.0/Workflow.html#File
type File struct {
	Class          string  `json:"class"`          // always "File"
	Location       string  `json:"location"`       // path to file (same as `path`)
	Path           string  `json:"path"`           // path to file
	Basename       string  `json:"basename"`       // last element of location path
	NameRoot       string  `json:"nameroot"`       // basename without file extension
	NameExt        string  `json:"nameext"`        // file extension of basename
	Contents       string  `json:"contents"`       // first 64 KiB of file as a string, if loadContents is true
	SecondaryFiles []*File `json:"secondaryFiles"` // array of secondaryFiles
}

// instantiates a new file object given a filepath
// returns pointer to the new File object
// presently loading both `path` and `location` for sake of loading all potentially needed context to js vm
// right now they hold the exact same path
func getFileObj(path string) (fileObj *File) {
	base, root, ext := getFileFields(path)
	fileObj = &File{
		Class:    "File",
		Location: path,
		Path:     path,
		Basename: base,
		NameRoot: root,
		NameExt:  ext,
	}
	return fileObj
}

// pedantic splitting regarding leading periods in the basename
// see: https://www.commonwl.org/v1.0/Workflow.html#File
// the description of nameroot and nameext
func getFileFields(path string) (base string, root string, ext string) {
	base = GetLastInPath(path)
	baseNoLeadingPeriods, nPeriods := trimLeading(base, ".")
	tmp := strings.Split(baseNoLeadingPeriods, ".")
	if len(tmp) == 1 {
		// no file extension
		root = tmp[0]
		ext = ""
	} else {
		root = strings.Join(tmp[:len(tmp)-1], ".")
		ext = "." + tmp[len(tmp)-1]
	}
	// add back any leading periods that were trimmed from base
	root = strings.Repeat(".", nPeriods) + root
	return base, root, ext
}

// given a string s and a character char
// count number of leading char's
// return s trimmed of leading char and count number of char's trimmed
func trimLeading(s string, char string) (suffix string, count int) {
	count = 0
	prevChar := char
	for i := 0; i < len(s) && prevChar == char; i++ {
		prevChar = string(s[i])
		if prevChar == char {
			count++
		}
	}
	suffix = strings.TrimLeft(s, char)
	return suffix, count
}

// creates File object for secondaryFile and loads into fileObj.SecondaryFiles field
// unsure of where/what to check here to potentially return an error
func (fileObj *File) loadSFilesFromPattern(suffix string, carats int) (err error) {
	path := fileObj.Location
	// however many chars there are
	// trim that number of file extentions from the end of the path
	for i := 0; i < carats; i++ {
		tmp := strings.Split(path, ".") // split path at periods
		tmp = tmp[:len(tmp)-1]          // exclude last file extension
		path = strings.Join(tmp, ".")   // reconstruct path without last file extension
	}
	path = path + suffix // append suffix (which is the original pattern with leading carats trimmed)

	// check whether file exists
	fileExists, err := exists(path)
	// HERE - TODO - decide how to handle case of secondaryFiles that don't exist - warning or error? still append file obj to list or not?
	// see: https://www.commonwl.org/v1.0/Workflow.html#WorkflowOutputParameter
	switch {
	case fileExists:
		// the secondaryFile exists
		fmt.Printf("\tfound secondary file %v\n", path)

		// construct file object for this secondary file
		sFile := getFileObj(path)

		// append this secondary file
		fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFile)

	case !fileExists:
		// the secondaryFile does not exist
		// if anything, this should be a warning - not an error
		// presently in this case, the secondaryFile object does NOT get appended to fileObj.SecondaryFiles
		fmt.Printf("\tWARNING: secondary file %v not found\n", path)
	}
	return nil
}

// loads contents of file into the File.Contents field
// NOTE: need handle prefix issue
func (fileObj *File) loadContents() (err error) {
	// HERE TODO same path prefix issue as in Glob() needs to be handled
	// prefix depends bucket mount location in workflow engine container and folder structure of bucket
	prefix := ""
	r, err := os.Open(prefix + fileObj.Location)
	if err != nil {
		return err
	}
	// read up to 64 KiB from file, as specified in CWL docs
	// 1 KiB is 1024 bytes -> 64 KiB is 65536 bytes
	contents := make([]byte, 65536, 65536)
	_, err = r.Read(contents)
	if err != nil {
		fmt.Printf("error reading file contents: %v", err)
		return err
	}
	// trim trailing null bytes if less than 65536 bytes were read
	contents = bytes.TrimRight(contents, "\u0000")

	// populate File.Contents field with contents
	fileObj.Contents = string(contents)
	return nil
}

// determines whether a map i represents a CWL file object
// note: since objects of type File are not maps, they return false -> unfortunate, but not a critical problem
// ----- maybe do some renaming to clear this up
func isFile(i interface{}) (f bool) {
	iType := reflect.TypeOf(i)
	iKind := iType.Kind()
	if iKind == reflect.Map {
		iMap := reflect.ValueOf(i)
		for _, key := range iMap.MapKeys() {
			if key.Type() == reflect.TypeOf("") {
				if key.String() == "class" {
					if iMap.MapIndex(key).Interface() == "File" {
						f = true
					}
				}
			}
		}
	}
	return f
}

// exists returns whether the given file or directory exists
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

// get path from a file object which is not of type File
// NOTE: maybe shouldn't be an error if no path but the contents field is populated
func GetPath(i interface{}) (path string, err error) {
	iter := reflect.ValueOf(i).MapRange()
	for iter.Next() {
		key, val := iter.Key().String(), iter.Value()
		if key == "location" || key == "path" {
			return val.Interface().(string), nil
		}
	}
	return "", fmt.Errorf("no location or path specified")
}
