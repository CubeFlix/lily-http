// server/server.go
// Lily HTTP server.

package server

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cubeflix/lily/client"
	"github.com/cubeflix/lily/network"
	"github.com/google/uuid"
)

// Drive data object.
type DriveData struct {
	Server  string
	Name    string
	Version string
	Drives  []interface{}
}

// Login page struct.
type LoginPageData struct {
	Error    string
	Redirect string
}

// Directory page struct.
type DirectoryPage struct {
	CurrentPath string
	Previous    string
	Dirs        []DirItem
	Files       []DirItem
}

// Directory item struct.
type DirItem struct {
	Path       string
	Name       string
	LastEditor string
	LastEdited time.Time
}

// Server variables.
var insecureSkipVerify bool = true
var host = "10.211.3.204"
var port = 8080
var lilyHost = "10.211.3.204"
var lilyPort = 42069

//go:embed static/drive.html
var homePage string

//go:embed static/login.html
var loginPage string

//go:embed static/dir.html
var dirPage string

// Get authentication.
func getAuth(r *http.Request) (string, uuid.UUID, error) {
	username, err := r.Cookie("username")
	if err != nil {
		return "", uuid.Nil, errors.New("no username")
	}
	id, err := r.Cookie("sessionID")
	if err != nil {
		return "", uuid.Nil, errors.New("no session id")
	}
	sid, err := uuid.Parse(id.Value)
	if err != nil {
		return "", uuid.Nil, err
	}
	return username.Value, sid, nil
}

// Home page.
func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/drive", http.StatusFound)
}

// Drive/filesystem page.
func driveHandler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/drive") {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	splitPath := strings.Split(r.URL.Path, "/")[2:]
	driveName := splitPath[0]
	path := splitPath[1:]

	// Get authentication.
	username, sessionID, err := getAuth(r)
	if err != nil {
		// Display auth page.
		http.Redirect(w, r, "/login?redirect=/drive", http.StatusFound)
		return
	}
	bytes, err := sessionID.MarshalBinary()
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %v", err), http.StatusInternalServerError)
		return
	}

	// Create the new client.
	c := client.NewClient(lilyHost, lilyPort, "", "", insecureSkipVerify, false) // TODO: insecure skip verify

	// Render default page.
	if driveName == "" {
		resp, err := c.MakeNonChunkRequest(*client.NewRequest(client.NewSessionAuth(username, bytes), "info", map[string]interface{}{}, time.Second*5))
		if err != nil {
			http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
			return
		}
		if resp.Code != 0 {
			if resp.Code == 6 {
				// Display auth page.
				http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed: %d %s", resp.Code, resp.String), http.StatusInternalServerError)
			return
		}

		// Display drive list.
		drives := resp.Data["drives"].([]interface{})
		t, err := template.New("drive").Parse(homePage)
		if err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
		t.Execute(w, DriveData{Server: fmt.Sprintf("%s:%d", host, port), Name: resp.Data["name"].(string), Version: resp.Data["version"].(string), Drives: drives})
		return
	}

	if len(splitPath) > 0 {
		// Display directory or file.
		resp, err := c.MakeNonChunkRequest(*client.NewRequest(client.NewSessionAuth(username, bytes), "stat", map[string]interface{}{"paths": []string{strings.Join(path, "/")}, "drive": driveName}, time.Second*5))
		if err != nil {
			http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
			return
		}
		if resp.Code != 0 {
			if resp.Code == 6 {
				// Display auth page.
				http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed: %d %s", resp.Code, resp.String), http.StatusInternalServerError)
			return
		}

		// Stat the path and check if we are at a drive or file.
		isFile := resp.Data["stat"].(map[string]interface{})[strings.Join(path, "/")].(map[string]interface{})["isfile"].(bool)
		if exists, _ := resp.Data["stat"].(map[string]interface{})[strings.Join(path, "/")].(map[string]interface{})["exists"].(bool); !exists {
			http.Error(w, "does not exist", 404)
			return
		}
		if isFile {
			// Display a file.
			conn, err := c.MakeConnection(true)
			if err != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}
			stream, err := c.SendRequestData(conn, *client.NewRequest(client.NewSessionAuth(username, bytes), "readfiles", map[string]interface{}{"paths": []string{strings.Join(path, "/")}, "drive": driveName}, time.Second*5), time.Second*5, true)
			if err != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}
			if err := c.ReceiveHeader(stream, time.Second*5); err != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}

			// Receive the chunks.
			ch := network.NewChunkHandler(stream)

			// Receive the chunk info.
			chunkInfo, err := ch.GetChunkRequestInfo(time.Second * 5)
			if err != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}
			for i := range chunkInfo {
				for n := 0; n < chunkInfo[i].NumChunks; n++ {
					_, size, err := ch.GetChunkInfo(time.Second * 5)
					if err != nil {
						http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
						return
					}
					buf := make([]byte, size)
					err = ch.GetChunk(&buf, time.Second*5)
					if err != nil {
						http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
						return
					}
					_, err = w.Write(buf)
					if err != nil {
						http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
						return
					}
				}
			}
			if ch.GetFooter(time.Second*5) != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}
			resp, err := c.ReceiveResponse(stream, time.Second*5)
			if err != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}
			if resp.Code != 0 {
				if resp.Code == 6 {
					// Display auth page.
					http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed: %d %s", resp.Code, resp.String), http.StatusInternalServerError)
				return
			}
		} else {
			// Display a directory.
			resp, err := c.MakeNonChunkRequest(*client.NewRequest(client.NewSessionAuth(username, bytes), "listdir", map[string]interface{}{"drive": driveName, "path": strings.Join(path, "/")}, time.Second*5))
			if err != nil {
				http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
				return
			}
			if resp.Code != 0 {
				if resp.Code == 6 {
					// Display auth page.
					http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed: %d %s", resp.Code, resp.String), http.StatusInternalServerError)
				return
			}

			split := strings.Split(r.URL.EscapedPath(), "/")
			listDir := DirectoryPage{CurrentPath: r.URL.EscapedPath(), Files: []DirItem{}, Dirs: []DirItem{}, Previous: filepath.ToSlash(filepath.Join(split[:len(split)-1]...))}

			// Generate the data for each item in the directory and sort.
			pathStatus := resp.Data["list"].([]interface{})
			for i := range pathStatus {
				currentPathStatus := pathStatus[i].(map[string]interface{})
				isFile := currentPathStatus["isfile"].(bool)
				pathInfo := DirItem{
					Name:       currentPathStatus["name"].(string),
					Path:       filepath.ToSlash(filepath.Join(r.URL.EscapedPath(), currentPathStatus["name"].(string))),
					LastEditor: currentPathStatus["lasteditor"].(string),
					LastEdited: time.Unix(currentPathStatus["lastedittime"].(int64), 0),
				}
				if isFile {
					listDir.Files = append(listDir.Files, pathInfo)
				} else {
					listDir.Dirs = append(listDir.Dirs, pathInfo)
				}
			}
			sort.Sort(ByCase(listDir.Dirs))
			sort.Sort(ByCase(listDir.Files))

			t, err := template.New("dir").Parse(dirPage)
			if err != nil {
				http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
				return
			}
			t.Execute(w, listDir)
		}
	}
}

// Handle requests to the login page.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/login" {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	switch r.Method {
	case "GET":
		// Display login page.
		t, err := template.New("drive").Parse(loginPage)
		if err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
		t.Execute(w, LoginPageData{Error: r.URL.Query().Get("error"), Redirect: r.URL.Query().Get("redirect")})
	case "POST":
		// Perform login.
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprintf("error: %v", err), http.StatusInternalServerError)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		c := client.NewClient(lilyHost, lilyPort, "", "", insecureSkipVerify, false) // TODO: insecure skip verify
		resp, err := c.MakeNonChunkRequest(*client.NewRequest(client.NewUserAuth(username, password), "login", map[string]interface{}{}, time.Second*5))
		if err != nil {
			http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
			return
		}
		if resp.Code != 0 {
			if resp.Code == 6 {
				// Display auth page.
				http.Redirect(w, r, "/login?redirect="+r.URL.Path+"&error="+resp.String, http.StatusFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed: %d %s", resp.Code, resp.String), http.StatusInternalServerError)
			return
		}
		id := resp.Data["id"].([]byte)
		uuid, err := uuid.FromBytes(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("error: %v", err), http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "username", Value: username})
		http.SetCookie(w, &http.Cookie{Name: "sessionID", Value: uuid.String()})
		http.Redirect(w, r, r.URL.Query().Get("redirect"), http.StatusFound)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// Handle requests to the logout page.
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logout" {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	// Perform logout.
	username, sid, err := getAuth(r)
	if err != nil {
		http.SetCookie(w, &http.Cookie{
			Name:    "username",
			Expires: time.Unix(0, 0),
		})
		http.SetCookie(w, &http.Cookie{
			Name:    "sessionID",
			Expires: time.Unix(0, 0),
		})
		http.Error(w, "error: "+err.Error(), http.StatusInternalServerError)
	}
	bytes, err := sid.MarshalBinary()
	if err != nil {
		http.SetCookie(w, &http.Cookie{
			Name:    "username",
			Expires: time.Unix(0, 0),
		})
		http.SetCookie(w, &http.Cookie{
			Name:    "sessionID",
			Expires: time.Unix(0, 0),
		})
		http.Error(w, "error: "+err.Error(), http.StatusInternalServerError)
	}
	c := client.NewClient(lilyHost, lilyPort, "", "", insecureSkipVerify, false) // TODO: insecure skip verify
	resp, err := c.MakeNonChunkRequest(*client.NewRequest(client.NewSessionAuth(username, bytes), "logout", map[string]interface{}{}, time.Second*5))
	if err != nil {
		http.Error(w, fmt.Sprintf("connection error: %v", err), http.StatusInternalServerError)
		return
	}
	if resp.Code != 0 {
		if resp.Code == 6 {
			// Display auth page.
			http.Redirect(w, r, "/login?error=Logged+out+successfully.", http.StatusFound)
			return
		}
		http.Error(w, fmt.Sprintf("failed: %d %s", resp.Code, resp.String), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "username",
		Expires: time.Unix(0, 0),
	})
	http.SetCookie(w, &http.Cookie{
		Name:    "sessionID",
		Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/login?error=Logged+out+successfully.", http.StatusFound)
}

func Serve(insecure bool, serverHost string, serverPort int, lilyServerHost string, lilyServerPort int, certFile, keyFile string) {
	insecureSkipVerify = insecure
	host = serverHost
	port = serverPort
	lilyHost = lilyServerHost
	lilyPort = lilyServerPort

	if certFile == "" {
		log.Fatal("no certificate file provided")
	}
	if keyFile == "" {
		log.Fatal("no key file provided")
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/drive/", driveHandler)

	log.Println("starting server at", fmt.Sprintf("%s:%d", host, port))
	if err := http.ListenAndServeTLS(fmt.Sprintf("%s:%d", host, port), certFile, keyFile, nil); err != nil {
		log.Fatal(err)
	}
}
