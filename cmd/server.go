package cmd

import (
	"bytes"
	"image"
	"image/jpeg"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/spf13/cobra"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts a webservice that takes screenshots",
	Long: `Start a webservice that takes screenshots.
	
The server starts its own webserver, and when invoked with the url query parameter,
instructs the underlying Chrome instance to take a screenshot and return it as
the HTTP response.

NOTE: When changing the server address to something other than localhost, make 
sure that only authorised connections can be made to the server port. By default,
access is restricted to localhost to reduce the risk of SSRF attacks against the
host or hosting infrastructure (AWS/Azure/GCP, etc). Consider strict IP filtering
or fronting this server with an authentication aware reverse proxy.

Allowed URLs, by default, need to start with http:// or https://. If you need
this restriction lifted, add the --allow-insecure-uri / -A flag. A word of 
warning though, that also means that someone may request a URL like file:///etc/passwd.

Assuming the server is hosted on localhost, an HTTP GET request to
take a screenshot of google.com would be:
	http://localhost:7171/?url=https://www.google.com
	
Optionally the request supports resizing to fit given width and height in request. This
keeps the original viewport of chrome equal to resolution given in program arguments.
	http://localhost:7171/?url=https://www.google.com&width=1280&height=720`,
	Example: `$ gowitness server
$ gowitness server --addr 0.0.0.0:8080`,
	Run: func(cmd *cobra.Command, args []string) {
		log := options.Logger

		if !strings.Contains(options.ServerAddr, "localhost") {
			log.Warn().Msg("exposing this server to other networks is dangerous! see the server command help for more information")
		}

		http.HandleFunc("/", handler)
		log.Info().Str("address", options.ServerAddr).Msg("server listening")
		if err := http.ListenAndServe(options.ServerAddr, nil); err != nil {
			log.Fatal().Err(err).Msg("webserver failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().StringVarP(&options.ServerAddr, "address", "a", "localhost:7171", "server listening address")
	serverCmd.Flags().BoolVarP(&options.AllowInsecureURIs, "allow-insecure-uri", "A", false, "allow uris that dont start with http(s)")
}

// handler is the HTTP handler for the web service this command exposes
func handler(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		http.Error(w, "url parameter missing. eg ?url=https://google.com", http.StatusNotAcceptable)
		return
	}

	var width = 0
	var height = 0

	width, err := strconv.Atoi(r.URL.Query().Get("width"))
	if err != nil || width < 1 {
		width = 0
	}

	height, err = strconv.Atoi(r.URL.Query().Get("height"))
	if err != nil || height < 1 {
		height = 0
	}

	if width > 0 && height == 0 {
		http.Error(w, "received width option without height argument", http.StatusNotAcceptable)
		return
	}

	if height > 0 && width == 0 {
		http.Error(w, "received height option without width argument", http.StatusNotAcceptable)
		return
	}

	url, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !options.AllowInsecureURIs {
		if !strings.HasPrefix(url.Scheme, "http") {
			http.Error(w, "only http(s) urls are accepted", http.StatusNotAcceptable)
			return
		}
	}

	buf, err := chrm.Screenshot(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if width == 0 {
		w.Header().Set("Content-Type", "image/png")
		w.Write(buf)
		return
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dstImage := imaging.Fit(img, width, height, imaging.Lanczos)

	w.Header().Set("Content-Type", "image/jpeg")
	err = jpeg.Encode(w, dstImage, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
