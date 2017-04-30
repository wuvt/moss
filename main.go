package main

import (
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var libpath = flag.String("library-path", "/tmp/library", "Path of library")
var apiuser = flag.String("apiuser", "admin", "API username")
var apikey = flag.String("apikey", "hunter2", "API key")
var port = flag.Int("port", 8080, "Port to listen on")
var configPath = flag.String("config", "", "Path to JSON config file")

var config Config

func checkAuth(w http.ResponseWriter, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(config.ApiUser)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(config.ApiKey)) != 1 {
		http.Error(w, "API key is incorrect", http.StatusUnauthorized)
		log.Println("Authentication failure for " + user)
		return false
	}
	return true
}

type Shard struct {
	MinUUID  string
	MaxUUID  string
	Writable bool
}

type Config struct {
	Port        int
	ApiUser     string
	ApiKey      string
	LibraryPath string
	Shards      []Shard
}

type ServerInfo struct {
	Version   string
	FreeSpace uint64
	Shards    []Shard
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Only GET is allowed", http.StatusNotImplemented)
		return
	}

	var stat syscall.Statfs_t
	syscall.Statfs(config.LibraryPath, &stat)
	freeSpace := stat.Bavail * uint64(stat.Bsize)

	serverInfo := ServerInfo{"git", freeSpace, config.Shards}
	js, err := json.Marshal(serverInfo)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	return
}

func listAllHandler(w http.ResponseWriter, r *http.Request) {
	uuidList := []string{}
	dirEnts, err := ioutil.ReadDir(config.LibraryPath)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, dirEnt := range dirEnts {
		if dirEnt.IsDir() {
			shardPath := path.Join(config.LibraryPath, dirEnt.Name())
			uuidEnts, err := ioutil.ReadDir(shardPath)
			if err != nil {
				log.Println(err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, uuidEnt := range uuidEnts {
				uuidList = append(uuidList, uuidEnt.Name())
			}
		}
	}
	js, err := json.Marshal(uuidList)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	params := strings.Split(r.URL.Path[len("/"):], "/")
	if len(params) == 0 || len(params[0]) == 0 {
		listAllHandler(w, r)
		return
	}
	params[0] = strings.ToLower(params[0])

	// Just to cut down on log spam
	if params[0] == "favicon.ico" {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	// At this point we assume that params[0] is a UUID
	uuid := params[0]

	switch r.Method {
	case "GET":
		if len(params) == 1 || (len(params) == 2 && params[1] == "") {
			listUUIDHandler(w, r, params)
			return
		} else {
			getHandler(w, r, params)
			return
		}
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
			return
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
	r := regexp.MustCompile("^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[8|9|a|b][a-f0-9]{3}-[a-f0-9]{12}$")
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
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	destPath := path.Join(uuidToPath(config.LibraryPath, uuid), "lock")

	if err := ensureSafePath(config.LibraryPath, destPath); err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if _, err := os.OpenFile(destPath, os.O_RDONLY|os.O_CREATE, 0644); err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Created lock\n")
}

func uuidToPath(basepath string, uuid string) string {
	shard := uuid[0:2]
	str := path.Join(basepath, shard, uuid)
	return str
}

func albumArtUploadHandler(w http.ResponseWriter, r *http.Request, uuid string) {
	err := uuidSanityCheck(uuid)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	destPath := path.Join(uuidToPath(config.LibraryPath, uuid), "albumart")

	if err := ensureSafePath(config.LibraryPath, destPath); err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if err := ioutil.WriteFile(destPath, body, 0644); err != nil {
		log.Println(err.Error())
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

func ensureSafePath(basepath string, targetpath string) error {
	abs, err := filepath.Abs(targetpath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(abs, basepath) {
		return &pathTraversalError{basepath, targetpath}
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
		log.Println(err.Error())
		return
	}

	lockPath := path.Join(uuidToPath(config.LibraryPath, uuid), "lock")
	if _, err := os.Stat(lockPath); err == nil {
		// Lock exists, refuse upload
		lerr := &lockExistsError{uuid}
		log.Println(lerr.Error())
		http.Error(w, lerr.Error(), http.StatusLocked)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	destPath := path.Join(uuidToPath(config.LibraryPath, uuid), "music", strings.Join(params[2:], "/"))

	if err := ensureSafePath(config.LibraryPath, destPath); err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	dir, _ := filepath.Split(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := ioutil.WriteFile(destPath, body, 0644); err != nil {
		log.Println(err.Error())
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

type Holding struct {
	FileList   []string
	HasArtwork bool
	Locked     bool
}

func listUUIDHandler(w http.ResponseWriter, r *http.Request, params []string) {
	err := uuidSanityCheck(params[0])
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uuidDir := uuidToPath(config.LibraryPath, params[0])
	if !dirExists(uuidDir) {
		http.Error(w, "holding not found on disk", http.StatusNotFound)
		log.Println("Holding not found: " + params[0])
		return
	}

	searchDir := path.Join(uuidDir, "music")
	fileList := []string{}
	err = filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if err != nil || f.IsDir() {
			return nil
		}
		fileList = append(fileList, path[len(searchDir)+1:])
		return nil
	})
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var hasArtwork bool
	var hasLock bool

	if _, err = os.Stat(path.Join(uuidDir, "albumart")); err != nil {
		hasArtwork = false
	} else {
		hasArtwork = true
	}

	if _, err = os.Stat(path.Join(uuidDir, "lock")); err != nil {
		hasLock = false
	} else {
		hasLock = true
	}

	holding := Holding{fileList, hasArtwork, hasLock}
	js, err := json.Marshal(holding)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	return
}

func getHandler(w http.ResponseWriter, r *http.Request, params []string) {
	err := uuidSanityCheck(params[0])
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uuidDir := uuidToPath(config.LibraryPath, params[0])
	if !dirExists(uuidDir) {
		http.Error(w, "holding not found on disk", http.StatusNotFound)
		log.Println("Holding not found: " + params[0])
		return
	}

	if params[1] == "albumart" {
		fp := path.Join(uuidDir, "albumart")
		if err := ensureSafePath(config.LibraryPath, fp); err != nil {
			log.Println(err.Error())
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
	if *configPath != "" {
		file, err := os.Open(*configPath)
		if err != nil {
			log.Fatal("Cannot open config file")
		}
		decoder := json.NewDecoder(file)
		config = Config{}
		err = decoder.Decode(&config)
		if err != nil {
			log.Fatal("Invalid config file")
		}
	} else {
		config.ApiUser = *apiuser
		config.ApiKey = *apikey
		config.Port = *port
		config.LibraryPath = *libpath

		// Config file is required for configurable shards
		config.Shards = []Shard{Shard{"00000000-0000-0000-0000-000000000000", "ffffffff-ffff-ffff-ffff-ffffffffffff", true}}
	}
	log.Println("Server running on port " + strconv.Itoa(config.Port))

	mux := http.NewServeMux()
	mux.HandleFunc("/version", versionHandler)
	mux.HandleFunc("/", mainHandler)
	http.ListenAndServe(":"+strconv.Itoa(config.Port), mux)
}
