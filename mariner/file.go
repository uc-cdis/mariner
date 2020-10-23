package mariner

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// this file contains code for handling/processing file objects

// File type represents a CWL file object
// NOTE: the json representation of field names is what gets loaded into js vm
// ----- see PreProcessContext() and accompanying note of explanation.
// ----- these json aliases are the fieldnames defined by cwl for cwl File objects
//
// see: see: https://www.commonwl.org/v1.0/Workflow.html#File
//
// would be nice for logging to strip some of the redundant information
// e.g., only have Class, Path, Contents, and SecondaryFiles
// omitempty
// but can't do that JSON encoding directly here because
// these JSON encodings are used for context in parameters refs and JS expressions
// so again - CANNOT implement the stripped JSON marhsalling here
// --- would need some preprocessing step before writing/storing a file object to log
// --- could just create a wrapper around the File type,
// --- like FileLog or something, which implements the desired, stripped JSON encodings
type File struct {
	Class          string  `json:"class"`          // always CWLFileType
	Location       string  `json:"location"`       // path to file (same as `path`)
	Path           string  `json:"path"`           // path to file
	Basename       string  `json:"basename"`       // last element of location path
	NameRoot       string  `json:"nameroot"`       // basename without file extension
	NameExt        string  `json:"nameext"`        // file extension of basename
	DirName        string  `json:"dirname"`        // name of directory containing the file
	Contents       string  `json:"contents"`       // first 64 KiB of file as a string, if loadContents is true
	SecondaryFiles []*File `json:"secondaryFiles"` // array of secondaryFiles
	// S3Key          string  `json:"-"`
}

// instantiates a new file object given a filepath
// returns pointer to the new File object
// presently loading both `path` and `location` for sake of loading all potentially needed context to js vm
// right now they hold the exact same path
// prefixissue - don't need to handle here - the 'path' argument is the full path, with working dir and all
func fileObject(path string) (fileObj *File) {
	base, root, ext, dirname := fileFields(path)
	fileObj = &File{
		Class:    CWLFileType,
		Location: path, // stores the full path
		Path:     path,
		Basename: base,
		NameRoot: root,
		NameExt:  ext,
		DirName:  dirname,
	}
	return fileObj
}

// pedantic splitting regarding leading periods in the basename
// see: https://www.commonwl.org/v1.0/Workflow.html#File
// the description of nameroot and nameext
func fileFields(path string) (base string, root string, ext string, dirname string) {
	base = lastInPath(path)
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
	dirname = strings.TrimSuffix(path, fmt.Sprintf("/%v", base))
	return base, root, ext, dirname
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
func (tool *Tool) loadSFilesFromPattern(fileObj *File, suffix string, carats int) (err error) {
	tool.Task.infof("begin load secondaryFiles from pattern for file: %v", fileObj.Path)

	path := fileObj.Location // full path -> no need to handle prefix issue here
	// however many chars there are
	// trim that number of file extentions from the end of the path
	for i := 0; i < carats; i++ {
		tmp := strings.Split(path, ".") // split path at periods
		tmp = tmp[:len(tmp)-1]          // exclude last file extension
		path = strings.Join(tmp, ".")   // reconstruct path without last file extension
	}
	path = path + suffix // append suffix (which is the original pattern with leading carats trimmed)

	// check whether file exists
	// fixme: decide how to handle case of secondaryFiles that don't exist - warning or error? still append file obj to list or not?
	// see: https://www.commonwl.org/v1.0/Workflow.html#WorkflowOutputParameter
	fileExists, err := exists(path)
	switch {
	case fileExists:
		// the secondaryFile exists
		tool.Task.infof("found secondaryFile: %v", path)

		// construct file object for this secondary file
		sFile := fileObject(path)

		// append this secondary file
		fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFile)

	case !fileExists:
		// the secondaryFile does not exist
		// if anything, this should be a warning - not an error
		// presently in this case, the secondaryFile object does NOT get appended to fileObj.SecondaryFiles
		tool.Task.warnf("secondaryFile not found: %v", path)
	}
	tool.Task.infof("end load secondaryFiles from pattern for file: %v", fileObj.Path)
	return nil
}

func (engine *K8sEngine) localPathToS3Key(path string) string {
	return strings.Replace(path, engineWorkspaceVolumeName, engine.UserID, 1)
}

// loads contents of file into the File.Contents field
// #no-fuse - read from s3, not locally
func (engine *K8sEngine) loadContents(f *File) (err error) {

	sess := engine.S3FileManager.newS3Session()
	downloader := s3manager.NewDownloader(sess)

	// Location field stores full path, no need to handle prefix here
	s3Key := engine.localPathToS3Key(f.Location)

	// Create a buffer to write the S3 Object contents to.
	// see: https://stackoverflow.com/questions/41645377/golang-s3-download-to-buffer-using-s3manager-downloader
	buf := &aws.WriteAtBuffer{}

	// read up to 64 KiB from file, as specified in CWL docs
	// 1 KiB is 1024 bytes -> 64 KiB is 65536 bytes
	//
	// S3 sdk supports specifying byte ranges
	// in this way: https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.35

	// Write the contents of S3 Object to the buffer
	s3Obj := &s3.GetObjectInput{
		Bucket: aws.String(engine.S3FileManager.S3BucketName),
		Key:    aws.String(s3Key),
		Range:  aws.String(fmt.Sprintf("bytes=%v-%v", 0, 65536)),
	}
	_, err = downloader.Download(buf, s3Obj)
	if err != nil {
		return fmt.Errorf("failed to download file, %v", err)
	}

	// populate File.Contents field with contents
	f.Contents = string(buf.Bytes())
	return nil
}

func (f *File) delete() error {
	err := os.Remove(f.Location)
	return err
}

// determines whether a map i represents a CWL file object
// NOTE: since objects of type File are not maps, they return false -> unfortunate, but not a critical problem
// ----- maybe do some renaming to clear this up
// fixme - see conformancelib
func isFile(i interface{}) (f bool) {
	iType := reflect.TypeOf(i)
	iKind := iType.Kind()
	if iKind == reflect.Map {
		iMap := reflect.ValueOf(i)
		for _, key := range iMap.MapKeys() {
			if key.Type() == reflect.TypeOf("") {
				if key.String() == "class" {
					if iMap.MapIndex(key).Interface() == CWLFileType {
						f = true
					}
				}
			}
		}
	}
	return f
}

func isArrayOfFile(i interface{}) (f bool) {
	if reflect.TypeOf(i).Kind() == reflect.Array {
		s := reflect.ValueOf(i)
		f = true
		for j := 0; j < s.Len() && f; j++ {
			if !isFile(s.Index(j)) {
				f = false
			}
		}
	}
	return f
}

// returns whether the given file or directory exists
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
// fixme - see conformancelib
func filePath(i interface{}) (path string, err error) {
	iter := reflect.ValueOf(i).MapRange()
	for iter.Next() {
		key, val := iter.Key().String(), iter.Value()
		if key == "location" || key == "path" {
			return val.Interface().(string), nil
		}
	}
	return "", fmt.Errorf("no location or path specified")
}
