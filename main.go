package main

import (
	"crypto/sha1"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cavaliercoder/grab"
	"github.com/ulikunitz/xz"
)

type Cache struct {
	ID      string `xml:",attr"`
	Version int    `xml:",attr"`
}

type Caches struct {
	Caches []Cache `xml:"Cache"`
}

type ContentFile struct {
	Name           string `xml:",attr"`
	Size           int    `xml:",attr"`
	SHA1Hash       string `xml:",attr"`
	CompressedSize int    `xml:",attr"`
}

type CacheInfo struct {
	ContentFiles []ContentFile `xml:"ContentFile"`
}

var outPath string

func main() {
	flag.StringVar(&outPath, "outPath", ".", "The target path for the download operation")
	flag.Parse()

	downloadURL := flag.Arg(0)

	if downloadURL == "" {
		log.Println("No download URL passed.")
		return
	}

	cacheResp, err := http.Get(fmt.Sprintf("%v/caches.xml", downloadURL))

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	defer cacheResp.Body.Close()
	cacheList, err := ioutil.ReadAll(cacheResp.Body)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var caches Caches
	err = xml.Unmarshal(cacheList, &caches)

	if err != nil {
		log.Printf("Error parsing XML: %v\n", err)
		return
	}

	for _, cache := range caches.Caches {
		updateCache(downloadURL, &cache)
	}

	updateExe(downloadURL)
}

func updateCache(downloadURL string, cache *Cache) {
	infoResp, err := http.Get(fmt.Sprintf("%v/diff/%v/info.xml", downloadURL, cache.ID))

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	defer infoResp.Body.Close()
	infoList, err := ioutil.ReadAll(infoResp.Body)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var cacheInfo CacheInfo
	err = xml.Unmarshal(infoList, &cacheInfo)

	if err != nil {
		log.Printf("Error parsing XML: %v\n", err)
		return
	}

	client := grab.NewClient()

	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()

	for _, file := range cacheInfo.ContentFiles {
		suffix := ""
		isCompressed := false

		sum, err := sha1sum(filepath.Join(outPath, file.Name))

		if err == nil && sum == file.SHA1Hash {
			fmt.Printf("%v already exists\n", file.Name)
			continue
		}

		if file.CompressedSize != file.Size {
			isCompressed = true
			suffix = ".xz"
		}

		outFile := fmt.Sprintf("%v%v", filepath.Join(outPath, file.Name), suffix)

		req, _ := grab.NewRequest(
			outFile,
			fmt.Sprintf("%v/diff/%v/%v%v", downloadURL, cache.ID, file.Name, suffix))

		resp := client.Do(req)

	Loop:
		for {
			select {
			case <-t.C:
				fmt.Printf("\r%v: %v / %v (%.2f%%)",
					file.Name,
					resp.BytesComplete(),
					resp.Size,
					100*resp.Progress())

			case <-resp.Done:
				fmt.Printf("\r%v: %v / %v (%.2f%%)\n",
					file.Name,
					resp.BytesComplete(),
					resp.Size,
					100*resp.Progress())

				break Loop
			}
		}

		if isCompressed {
			f, err := os.Open(outFile)

			if err != nil {
				return
			}

			r, err := xz.NewReader(f)

			outF, err := os.Create(filepath.Join(outPath, file.Name))

			_, err = io.Copy(outF, r)

			f.Close()
			outF.Close()
			os.Remove(outFile)

			if err != nil {
				return
			}
		}
	}
}

func updateExe(downloadURL string) {
	client := grab.NewClient()
	req, _ := grab.NewRequest(
		filepath.Join(outPath, "FiveM.exe.xz"),
		fmt.Sprintf("%v/CitizenFX.exe.xz", downloadURL))

	resp := client.Do(req)

Loop:
	for {
		select {
		case <-resp.Done:
			break Loop
		}
	}

	f, _ := os.Open(filepath.Join(outPath, "FiveM.exe.xz"))

	r, _ := xz.NewReader(f)

	outF, _ := os.Create(filepath.Join(outPath, "FiveM.exe"))

	io.Copy(outF, r)

	f.Close()
	outF.Close()
	os.Remove(filepath.Join(outPath, "FiveM.exe.xz"))

	os.Create(filepath.Join(outPath, "FiveM.exe.formaldev"))
}

func sha1sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%X", h.Sum(nil)), nil
}
