package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
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

// Models
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

type Holding struct {
	FileList   []string
	HasArtwork bool
	Locked     bool
}

// Utils
func httpErrorHandler(msg string, code int) http.HandlerFunc {
	return func (w http.ResponseWriter, r *http.Request) {
		http.Error(w, msg, code)
	}
}

func uuidToPath(basepath string, uuid string) string {
	shard := uuid[0:2]
	str := path.Join(basepath, shard, uuid)
	return str
}

// Checks
func checkAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func (w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
		   subtle.ConstantTimeCompare([]byte(user), []byte(config.ApiUser)) != 1 ||
		   subtle.ConstantTimeCompare([]byte(pass), []byte(config.ApiKey)) != 1 {
			http.Error(w, "API key is incorrect", http.StatusUnauthorized)
			log.Println("Authentication failure for " + user)
			return
		}
		handler(w, r)
	}
}

func checkUUID(handler http.HandlerFunc) http.HandlerFunc {
	return func (w http.ResponseWriter, r *http.Request) {
		uuid := strings.ToLower(r.PathValue("uuid"))
		if len(uuid) != 36 {
			err_str := uuid + " - Invalid length"
			log.Println(err_str)
			http.Error(w, err_str, http.StatusBadRequest)
			return
		}
		re := regexp.MustCompile("^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[8|9|a|b][a-f0-9]{3}-[a-f0-9]{12}$")
		if !re.MatchString(uuid) {
			err_str := uuid + " - Invalid uuid4 format"
			log.Println(err_str)
			http.Error(w, err_str, http.StatusBadRequest)
			return
		}
		handler(w, r)
	}
}

func checkUUIDDir(handler http.HandlerFunc) http.HandlerFunc {
	return func (w http.ResponseWriter, r *http.Request) {
		uuid := strings.ToLower(r.PathValue("uuid"))
		uuidDir := uuidToPath(config.LibraryPath, uuid)

		stat, err := os.Stat(uuidDir)
		if err != nil {
			http.Error(w, "holding not found on disk", http.StatusNotFound)
			log.Println("Holding not found: " + uuid + "; " + err.Error())
			return
		}
		if !stat.Mode().IsDir() {
			http.Error(w, "holding not found on disk", http.StatusNotFound)
			log.Println("Holding not found: " + uuid + "; path not a dir")
			return
		}
		handler(w, r)
	}
}

func checkPath(handler http.HandlerFunc) http.HandlerFunc {
	return func (w http.ResponseWriter, r *http.Request) {
		uuid := strings.ToLower(r.PathValue("uuid"))
		uuidDir := uuidToPath(config.LibraryPath, uuid)
		file_path := r.PathValue("file_path")
		destPath := path.Join(uuidDir, "music", file_path)

		abs, err := filepath.Abs(destPath)
		if err != nil {
			log.Println(err.Error())
			http.Error(w, "Bad path: " + file_path, http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(abs, config.LibraryPath) {
			err_str := destPath + " is outside of " + config.LibraryPath
			log.Println(err_str)
			http.Error(w, err_str, http.StatusUnauthorized)
			return
		}
		handler(w,r)
	}
}

func checkLock(handler http.HandlerFunc) http.HandlerFunc {
	return func (w http.ResponseWriter, r *http.Request) {
		uuid := strings.ToLower(r.PathValue("uuid"))
		uuidDir := uuidToPath(config.LibraryPath, uuid)
		lockPath := path.Join(uuidDir, "lock")
		if _, err := os.Stat(lockPath); err == nil {
			// Lock exists, refuse upload
			lerr_str := "Lock exists for " + uuid
			log.Println(lerr_str)
			http.Error(w, lerr_str, http.StatusForbidden)
			return
		}
		handler(w,r)
	}
}

// Handlers
func versionHandler(w http.ResponseWriter, r *http.Request) {
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

func lockCreationHandler(w http.ResponseWriter, r *http.Request) {
	// Handle locking a UUID once all audio files are added. You can still add
	// album art after the fact. Locks must be removed manually as holdings are
	// supposed to be immutable once they are added.
	uuid := strings.ToLower(r.PathValue("uuid"))
	uuidDir := uuidToPath(config.LibraryPath, uuid)
	destPath := path.Join(uuidDir, "lock")

	if _, err := os.OpenFile(destPath, os.O_RDONLY|os.O_CREATE, 0644); err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Created lock\n")
}

func albumArtUploadHandler(w http.ResponseWriter, r *http.Request) {
	uuid := strings.ToLower(r.PathValue("uuid"))
	uuidDir := uuidToPath(config.LibraryPath, uuid)
	destPath := path.Join(uuidDir, "albumart")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
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

func trackUploadHandler(w http.ResponseWriter, r *http.Request) {
	uuid := strings.ToLower(r.PathValue("uuid"))
	uuidDir := uuidToPath(config.LibraryPath, uuid)
	file_path := r.PathValue("file_path")
	destPath := path.Join(uuidDir, "music", file_path)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func listUUIDHandler(w http.ResponseWriter, r *http.Request) {
	uuid := strings.ToLower(r.PathValue("uuid"))
	uuidDir := uuidToPath(config.LibraryPath, uuid)

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
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hasArtwork := true
	if _, err = os.Stat(path.Join(uuidDir, "albumart")); err != nil {
		hasArtwork = false
		if !errors.Is(err, os.ErrNotExist) {
			log.Println(err.Error())
		}
	}

	hasLock := true
	if _, err = os.Stat(path.Join(uuidDir, "lock")); err != nil {
		hasLock = false
		if !errors.Is(err, os.ErrNotExist) {
			log.Println(err.Error())
		}
	}

	holding := Holding{fileList, hasArtwork, hasLock}
	js, err := json.Marshal(holding)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	return
}

func albumArtDownloadHandler(w http.ResponseWriter, r *http.Request) {
	uuid := strings.ToLower(r.PathValue("uuid"))
	uuidDir := uuidToPath(config.LibraryPath, uuid)
	fp := path.Join(uuidDir, "albumart")
	http.ServeFile(w, r, fp)
}

func trackDownloadHandler(w http.ResponseWriter, r *http.Request) {
	uuid := strings.ToLower(r.PathValue("uuid"))
	file_path := r.PathValue("file_path")
	uuidDir := uuidToPath(config.LibraryPath, uuid)

	if len(file_path) == 0 {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	fs := http.FileServer(http.Dir(path.Join(uuidDir, "music")))
	sp := http.StripPrefix("/"+uuid+"/music", fs)
	sp.ServeHTTP(w, r)
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
		config.LibraryPath, err = filepath.Abs(config.LibraryPath)
		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		config.ApiUser = *apiuser
		config.ApiKey = *apikey
		config.Port = *port
		abs, err := filepath.Abs(*libpath)
		if err != nil {
			log.Fatal(err.Error())
		}
		config.LibraryPath = abs

		// Config file is required for configurable shards
		config.Shards = []Shard{Shard{"00000000-0000-0000-0000-000000000000", "ffffffff-ffff-ffff-ffff-ffffffffffff", true}}
	}

	mux := http.NewServeMux()

	// 'GET' also handles the HEAD method
	// API
	mux.HandleFunc("PUT /{uuid}/music/{file_path...}",
		checkAuth(checkUUID(checkLock(checkPath(trackUploadHandler)))),
	)
	mux.HandleFunc("GET /{uuid}/music/{file_path...}",
		checkUUID(checkUUIDDir(trackDownloadHandler)),
	)
	mux.HandleFunc("PUT /{uuid}/albumart/",
		checkAuth(checkUUID(checkUUIDDir(albumArtUploadHandler))),
	)
	mux.HandleFunc("GET /{uuid}/albumart/",
		checkUUID(checkUUIDDir(albumArtDownloadHandler)),
	)
	mux.HandleFunc("GET /{uuid}/{$}",
		checkUUID(checkUUIDDir(listUUIDHandler)),
	)
	mux.HandleFunc("PUT /{uuid}/lock/",
		checkAuth(checkUUID(checkUUIDDir(lockCreationHandler))),
	)
	mux.HandleFunc("GET /{$}", listAllHandler)
	mux.HandleFunc("GET /version/{$}", versionHandler)

	// Bad
	mux.HandleFunc("/", httpErrorHandler("", http.StatusMethodNotAllowed))
	mux.HandleFunc("GET /favicon.ico", http.NotFound)
	mux.HandleFunc("PUT /{_}/{$}",
		httpErrorHandler("Insufficient parameters", http.StatusBadRequest),
	)
	mux.HandleFunc("PUT /{uuid}/{_}/",
		checkAuth(httpErrorHandler("No request handler for that", http.StatusBadRequest)),
	)
	mux.HandleFunc("GET /{uuid}/{_}/",
		checkUUID(checkUUIDDir(httpErrorHandler("invalid url", http.StatusBadRequest))),
	)

	log.Println("Server running on port " + strconv.Itoa(config.Port))
	http.ListenAndServe(":"+strconv.Itoa(config.Port), mux)
}
