package main

import (
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var libpath = flag.String("library-path", "/tmp/library", "Path of library")

// TODO support a config file for auth
var apiuser = flag.String("apiuser", "admin", "API username")
var apikey = flag.String("apikey", "hunter2", "API key")

func checkAuth(w http.ResponseWriter, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(*apiuser)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(*apikey)) != 1 {
		http.Error(w, "API key is incorrect", http.StatusUnauthorized)
		return false
	}
	return true
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	params := strings.Split(r.URL.Path[len("/"):], "/")
	if len(params) == 0 {
		http.NotFound(w, r)
		return
	}
	uuid := params[0]

	switch r.Method {
	case "GET":
		getHandler(w, r, params)
		return
	case "HEAD":
		getHandler(w, r, params)
		return
	case "PUT":
		if !checkAuth(w, r) {
			return
		}
		if len(params) < 2 {
			http.Error(w, "Insufficient parameters", http.StatusBadRequest)
			return
		} else if params[1] == "lock" {
			lockCreationHandler(w, r, uuid)
			return
		} else if params[1] == "music" {
			trackUploadHandler(w, r, params)
			return
		} else if params[1] == "albumart" {
			albumArtUploadHandler(w, r, uuid)
			return
		} else {
			http.Error(w, "No request handler for that", http.StatusBadRequest)
		}
	default:
		http.Error(w, "", http.StatusNotImplemented)
		return
	}

}

type uuidError struct {
	uuid    string
	problem string
}

func (e *uuidError) Error() string {
	return fmt.Sprintf("%s - %s", e.uuid, e.problem)
}

func uuidSanityCheck(uuid string) error {
	if len(uuid) != 36 {
		return &uuidError{uuid, "Invalid length"}
	}
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$")
	if !r.MatchString(uuid) {
		return &uuidError{uuid, "Invalid uuid4 format"}
	}
	return nil
}

func lockCreationHandler(w http.ResponseWriter, r *http.Request, uuid string) {
	// Handle locking a UUID once all audio files are added. You can still add
	// album art after the fact. Locks must be removed manually as holdings are
	// supposed to be immutable once they are added.
	err := uuidSanityCheck(uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	destPath := path.Join(uuidToPath(libpath, uuid), "lock")

	if err := ensureSafePath(libpath, destPath); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if _, err := os.OpenFile(destPath, os.O_RDONLY|os.O_CREATE, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Created lock\n")
}

func uuidToPath(basepath *string, uuid string) string {
	shard := uuid[0:2]
	str := path.Join(*basepath, shard, uuid)
	return str
}

func albumArtUploadHandler(w http.ResponseWriter, r *http.Request, uuid string) {
	err := uuidSanityCheck(uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	destPath := path.Join(uuidToPath(libpath, uuid), "albumart")

	if err := ensureSafePath(libpath, destPath); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if err := ioutil.WriteFile(destPath, body, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "uploaded: %d bytes\n", len(body))
	return
}

type pathTraversalError struct {
	basePath   string
	targetPath string
}

func (e *pathTraversalError) Error() string {
	return fmt.Sprintf("%s is outside of %s", e.targetPath, e.basePath)
}

func ensureSafePath(basepath *string, targetpath string) error {
	abs, err := filepath.Abs(targetpath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(abs, *basepath) {
		return &pathTraversalError{*basepath, targetpath}
	}
	return nil
}

type lockExistsError struct {
	uuid string
}

func (e *lockExistsError) Error() string {
	return fmt.Sprintf("Lock exists for %s", e.uuid)
}

func trackUploadHandler(w http.ResponseWriter, r *http.Request, params []string) {
	uuid := params[0]
	err := uuidSanityCheck(uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		fmt.Println(err.Error())
		return
	}

	lockPath := path.Join(uuidToPath(libpath, uuid), "lock")
	if _, err := os.Stat(lockPath); err == nil {
		// Lock exists, refuse upload
		lerr := &lockExistsError{uuid}
		http.Error(w, lerr.Error(), http.StatusLocked)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	destPath := path.Join(uuidToPath(libpath, uuid), "music", strings.Join(params[2:], "/"))

	if err := ensureSafePath(libpath, destPath); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	dir, _ := filepath.Split(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := ioutil.WriteFile(destPath, body, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "uploaded: %d bytes\n", len(body))
	return

}

func dirExists(path string) bool {
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat.Mode().IsDir()
}

func getHandler(w http.ResponseWriter, r *http.Request, params []string) {
	err := uuidSanityCheck(params[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uuidDir := uuidToPath(libpath, params[0])
	if !dirExists(uuidDir) {
		http.Error(w, "holding not found on disk", http.StatusNotFound)
		return
	}

	if len(params) == 1 || (len(params) == 2 && len(params[1]) == 0) {
		searchDir := path.Join(uuidDir, "music")
		fileList := []string{}
		err := filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
			if err != nil || f.IsDir() {
				return nil
			}
			fileList = append(fileList, path[len(searchDir)+1:])
			return nil
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		js, err := json.Marshal(fileList)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
		return

	} else if params[1] == "albumart" {
		fp := path.Join(uuidDir, "albumart")
		if err := ensureSafePath(libpath, fp); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		http.ServeFile(w, r, fp)
		return

	} else if params[1] == "music" && len(params) >= 3 && len(params[2]) > 0 {
		fs := http.FileServer(http.Dir(path.Join(uuidDir, "music")))
		sp := http.StripPrefix("/"+params[0]+"/music", fs)
		sp.ServeHTTP(w, r)
		return

	} else {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return

	}

}

func main() {
	flag.Parse()
	mux := http.NewServeMux()

	mux.HandleFunc("/", mainHandler)
	http.ListenAndServe(":8080", mux)
}
