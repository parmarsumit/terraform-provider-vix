package helper

import (
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/dustin/go-humanize"
)

type FetchConfig struct {
	URL          string
	Checksum     string
	ChecksumType string
	DownloadPath string
}

func FetchFile(config FetchConfig) (string, error) {
	if config.URL == "" {
		panic("URL is required")
	}

	if config.Checksum == "" {
		panic("Checksum is required")
	}

	if config.ChecksumType == "" {
		panic("Checksum type is required")
	}

	if config.DownloadPath == "" {
		config.DownloadPath = os.TempDir()
	}

	u, err := url.Parse(config.URL)
	if err != nil {
		return "", err
	}

	_, filename := path.Split(u.Path)
	if filename == "" {
		filename = "unnamed"
	}

	os.MkdirAll(config.DownloadPath, 0740)

	filePath := filepath.Join(config.DownloadPath, filename)
	vmPath := filepath.Join(config.DownloadPath, config.Checksum)

	log.Printf("[DEBUG] Opening %s...", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("[DEBUG] %s file does not exist. Downloading it...", filename)

		data, err := download(config.URL)
		if err != nil {
			return "", err
		}

		file, err = write(data, filePath)
		if err != nil {
			return "", err
		}
		data.Close()
	}
	defer file.Close()

	// We need to make sure the reader is pointing to the beginning of the file
	// so verifying integrity does not fail
	file.Seek(0, 0)

	if err = VerifyChecksum(file, config.ChecksumType, config.Checksum); err != nil {
		log.Printf("[DEBUG] File on disk does not match current checksum.\n Downloading file again...")

		data, err := download(config.URL)
		if err != nil {
			return "", err
		}

		file, err = write(data, filePath)
		if err != nil {
			return "", err
		}
		data.Close()

		file.Seek(0, 0)
		if err = VerifyChecksum(file, config.ChecksumType, config.Checksum); err != nil {
			return "", err
		}
	}

	// If an unpacked VM folder does not exist or is empty then unpack image.
	_, err = os.Stat(vmPath)
	vmPathExist := err != nil && os.IsNotExist(err)

	// There is no need to get the error as the slice will be empty anyways
	finfo, _ := ioutil.ReadDir(vmPath)
	vmPathEmpty := len(finfo) == 0

	if !vmPathExist || vmPathEmpty {
		// TODO(c4milo): Make sure the file is a tgz file before attempting
		// to unpack it.
		_, err = UnpackFile(file, vmPath)
		if err != nil {
			return "", err
		}
	}

	return vmPath, nil
}

func write(reader io.Reader, filePath string) (*os.File, error) {
	log.Printf("[DEBUG] Downloading file data to %s", filePath)

	gzfile, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	written, err := io.Copy(gzfile, reader)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] %s written to %s", humanize.Bytes(uint64(written)), filePath)

	return gzfile, nil
}

func download(URL string) (io.ReadCloser, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}

	resp, err := client.Get(URL)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unable to fetch data, server returned code %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func UnpackFile(file *os.File, destPath string) (string, error) {
	os.MkdirAll(destPath, 0740)

	//unzip
	log.Printf("[DEBUG] Unzipping file stream ...")
	file.Seek(0, 0)

	unzippedFile, err := gzip.NewReader(file)
	if err != nil && err != io.EOF {
		return "", err
	}
	defer unzippedFile.Close()

	//untar
	return Untar(unzippedFile, destPath)
}
