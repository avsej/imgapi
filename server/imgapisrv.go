package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/trondn/imgapi/common"
	"github.com/trondn/imgapi/errorcodes"
)

var configuration common.Configuration

func sendResponse(w http.ResponseWriter, code int, content map[string]interface{}) {
	h := w.Header()
	h.Set("Server", "Norbye Public Images Repo")
	w.WriteHeader(code)
	if content != nil {
		h.Set("Content-Type", "application/json; charset=utf-8")
		a, _ := json.MarshalIndent(content, "", "  ")
		w.Write(a)
	}
}

/**
 * Split up the /images/:uuid/file URL
 */
func splitImagesUrl(url string) (uuid string, file string, err error) {
	if strings.Index(url, "/images/") != 0 {
		return uuid, file, errors.New("Invalid url")
	}

	// pick out the uuid
	uuid = url[8:] // everything after "/images/"
	if len(uuid) == 0 {
		return uuid, file, errors.New("Invalid url")
	}

	index := strings.Index(uuid, "/")
	file = ""
	if index != -1 {
		file = uuid[index:]
		uuid = uuid[0:index]
	}

	return uuid, file, nil
}

/*
Name	Endpoint	Notes
ListImages	GET /images	List available images.
GetImage	GET /images/:uuid	Get a particular image manifest.
GetImageFile	GET /images/:uuid/file	Get the file for this image.
GetImageIcon	GET /images/:uuid/icon	Get the image icon file.
*/

// Handle all GET request made to /images
func doHandleGetImages(w http.ResponseWriter, r *http.Request, params url.Values) {
	if r.URL.Path == "/images" {
		ListImages(configuration.Datadir, w, r)
		return
	}

	uuid, file, err := splitImagesUrl(r.URL.Path)
	if err != nil {
		sendResponse(w, errorcodes.InvalidParameter,
			map[string]interface{}{
				"code":    "InvalidParameter",
				"message": fmt.Sprintf("%v", err),
			})
		return
	}

	// check if the resource exists
	filename := configuration.Datadir + "/" + uuid
	_, err = os.Stat(filename)
	if err == nil {
		sendResponse(w, errorcodes.ResourceNotFound,
			map[string]interface{}{
				"code":    "ResourceNotFound",
				"message": fmt.Sprintf("Failed to locate %s: %v", filename, err),
			})
		return
	}

	// Ok, everything should be OK.. go do it!
	if len(file) == 0 {
		GetImage(w, r, params, filename)
		return
	}

	if file == "/icon" {
		GetImageIcon(w, r, params, filename)
		return
	}

	if file == "/file" {
		GetImageFile(w, r, params, filename)
		return
	}

	sendResponse(w, errorcodes.ResourceNotFound,
		map[string]interface{}{
			"code":    "ResourceNotFound",
			"message": "Requested resource does not exist",
		})
}

/*
Handle all DELETE request made to /images
DeleteImage	DELETE /images/:uuid	Delete an image (and its file).
DeleteImageIcon	DELETE /images/:uuid/icon	Remove the image icon.
*/
func doHandleDeleteImages(w http.ResponseWriter, r *http.Request, params url.Values) {
	uuid, file, err := splitImagesUrl(r.URL.Path)
	if err != nil {
		sendResponse(w, errorcodes.InvalidParameter,
			map[string]interface{}{
				"code":    "InvalidParameter",
				"message": "Failed to decode URL",
			})
		return
	}

	path := configuration.Datadir + "/" + uuid
	if len(file) > 0 {
		if file == "/icon" {
			DeleteImageIcon(w, r, params, path)
		} else {
			sendResponse(w, errorcodes.ResourceNotFound,
				map[string]interface{}{
					"code":    "ResourceNotFound",
					"message": "Resource does not exists",
				})
		}
	} else {
		DeleteImage(w, r, params, path)
	}
}

// Handle all POST request made to /images
/*
CreateImage	POST /images	Create a new (unactivated) image from a manifest.

ActivateImage	POST /images/:uuid?action=activate	Activate the image.
UpdateImage	POST /images/:uuid?action=update	Update image manifest fields. This is limited. Some fields are immutable.
DisableImage	POST /images/:uuid?action=disable	Disable the image.
EnableImage	POST /images/:uuid?action=enable	Enable the image.
ExportImage	POST /images/:uuid?action=export	Exports an image to the specified Manta path.
CopyRemoteImage	POST /images/$uuid?action=copy-remote&dc=us-west-1	NYI (IMGAPI-278) Copy one's own image from another DC in the same cloud.
AdminImportRemoteImage	POST /images/$uuid?action=import-remote&source=$imgapi-url	Import an image from another IMGAPI
AdminImportImage	POST /images/$uuid?action=import	Only for operators to import an image and maintain uuid and published_at.
ChannelAddImage	POST /images/:uuid?action=channel-add	Add an existing image to another channel.



AddImageAcl	POST /images/:uuid/acl?action=add	Add account UUIDs to the image ACL.
RemoveImageAcl	POST /images/:uuid/acl?action=remove	Remove account UUIDs from the image ACL.

AddImageIcon	POST /images/:uuid/icon	Add the image icon.

CreateImageFromVm	POST /images?action=create-from-vm	Create a new (activated) image from an existing VM.

*/
func doHandlePostImages(w http.ResponseWriter, r *http.Request, params url.Values) {
	if "/images" == r.URL.Path {
		CreateImage(w, r, params, configuration.Datadir)
		return
	}

	uuid, file, err := splitImagesUrl(r.URL.Path)
	if err != nil {
		sendResponse(w, errorcodes.InvalidParameter,
			map[string]interface{}{
				"code":    "InvalidParameter",
				"message": "Failed to decode URL",
			})
		return
	}

	path := configuration.Datadir + "/" + uuid
	_, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			sendResponse(w, errorcodes.ResourceNotFound,
				map[string]interface{}{
					"code":    "ResourceNotFound",
					"message": "Failed to locate resource",
				})
		} else {
			sendResponse(w, errorcodes.InternalError,
				map[string]interface{}{
					"code":    "InternalError",
					"message": fmt.Sprintf("Failed to locate resource %v", err),
				})
		}
		return
	}

	switch file {
	case "/icon":
		AddImageIcon(w, r, params, path)
		return

	case "/acl":
		sendResponse(w, errorcodes.InsufficientServerVersion,
			map[string]interface{}{
				"code":    "InsufficientServerVersion",
				"message": "acl is not implemented",
			})
		break

	case "": // the path just contains the UUID and optional parameters
		action, ok := params["action"]
		if ok {
			switch action[0] {
			case "activate":
				ActivateImage(w, r, params, path)
				break
			case "update":
				UpdateImage(w, r, params, path)
				break
			case "disable":
				DisableImage(w, r, params, path)
				break
			case "enable":
				EnableImage(w, r, params, path)
				break

			case "export":
			case "copy-remote":
			case "import-remote":
			case "import":
			case "channel-add":
				// Not implemented yet
				sendResponse(w, errorcodes.InsufficientServerVersion,
					map[string]interface{}{
						"code":    "InsufficientServerVersion",
						"message": fmt.Sprintf("action=\"%s\" is not implemented", action[0]),
					})
				break

			default:
				sendResponse(w, errorcodes.InvalidParameter,
					map[string]interface{}{
						"code":    "InvalidParameter",
						"message": fmt.Sprintf("Invalid action \"%s\"", action[0]),
					})
			}
		} else {
			sendResponse(w, errorcodes.InvalidParameter,
				map[string]interface{}{
					"code":    "InvalidParameter",
					"message": "action parameter not specified",
				})
		}
		return
	default:
		// The request was for an invalid resource
		sendResponse(w, errorcodes.ResourceNotFound,
			map[string]interface{}{
				"code":    "ResourceNotFound",
				"message": "Invalid URL specified",
			})
		return
	}
}

/*
 * Handle all PUT request made to /images
 *  AddImageFile	PUT /images/:uuid/file	Upload the image file.
 */
func doHandlePutImages(w http.ResponseWriter, r *http.Request, params url.Values) {
	uuid, file, err := splitImagesUrl(r.URL.Path)
	if err != nil || file != "/file" {
		sendResponse(w, errorcodes.InvalidParameter,
			map[string]interface{}{
				"code":    "InvalidParameter",
				"message": "Failed to decode URL",
			})
		return
	}

	path := configuration.Datadir + "/" + uuid
	AddImageFile(w, r, params, path)
}

/**
 * Handle all of the requests to "/images*" and dispatch the
 * request to the correct handler function.
 *
 * All operations that modify data _DO_ requre that the user
 * provides a username and password. (currently all users
 * have access to all commands)
 */
func doHandleImages(w http.ResponseWriter, r *http.Request) {
	authenticated := false

	username, password, ok := r.BasicAuth()
	if ok {
		found := false
		for i := 0; i < len(configuration.Userdb); i++ {
			entry := configuration.Userdb[i]
			if username != entry.Name {
				continue
			}

			found = true
			if password != entry.Password {
				log.Printf("Invalid username password combo for %s", username)
				sendResponse(w, errorcodes.UnauthorizedError,
					map[string]interface{}{
						"code":    "UnauthorizedError",
						"message": "Invalid username/password combination",
					})

				return
			}
		}

		if !found {
			log.Printf("User %s does not exists", username)
			sendResponse(w, errorcodes.AccountDoesNotExist,
				map[string]interface{}{
					"code":    "AccountDoesNotExist",
					"message": fmt.Sprintf("User %s does not exist", username),
				})
			return
		}

		authenticated = true
	}

	parameters, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		sendResponse(w, errorcodes.InternalError,
			map[string]interface{}{
				"code":    "InternalError",
				"message": "Failed to parse query",
			})
		return
	}
	if len(r.Method) == 0 || r.Method == "GET" {
		doHandleGetImages(w, r, parameters)
	} else if r.Method == "DELETE" {
		if authenticated {
			doHandleDeleteImages(w, r, parameters)
		} else {
			w.WriteHeader(errorcodes.UnauthorizedError)
		}
	} else if r.Method == "POST" {
		if authenticated {
			doHandlePostImages(w, r, parameters)
		} else {
			w.WriteHeader(errorcodes.UnauthorizedError)
		}
	} else if r.Method == "PUT" {
		if authenticated {
			doHandlePutImages(w, r, parameters)
		} else {
			w.WriteHeader(errorcodes.UnauthorizedError)
		}
	}
}

/*
AdminGetState	GET /state	Dump internal server state (for dev/debugging)
ListChannels	GET /channels	List image channels (if the server uses channels).
Ping	GET /ping	Ping if the server is up.
*/

func StartImageServer(conf common.Configuration) {
	configuration = conf

	_, err := os.Stat(configuration.Datadir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(configuration.Datadir, 0777)
		if err != nil {
			panic(fmt.Sprintf("Failed to create %s: %v", configuration.Datadir, err))
		}
	}

	http.HandleFunc("/images", doHandleImages)
	http.HandleFunc("/images/", doHandleImages)
	http.HandleFunc("/channels", ListChannels)
	http.HandleFunc("/ping", Ping)
	http.ListenAndServe(":"+strconv.Itoa(configuration.Port), nil)
}
