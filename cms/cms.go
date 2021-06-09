package cms

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var client = http.Client{
	Timeout: time.Second * 5, // Maximum of 5 secs
}

type Config struct {
	url          string
	assets       string
	imageUrl     string
	assetsImages string
	workDir      string
}

var config Config

func loadConfig() {
	config.url = viper.GetString("cms.url")
	config.assets = viper.GetString("cms.assets")
	config.imageUrl = viper.GetString("cms.image-url")
	config.assetsImages = viper.GetString("cms.assets-images")
}

func SyncCMS() {
	var err error

	loadConfig()
	log.Infof("Config: %+v", config)

	mediaLibraryRE = regexp.MustCompile(fmt.Sprintf("https?://%s", config.imageUrl))

	config.workDir, err = prepareDirectories()
	if err != nil {
		log.Fatal("prepare directories", err)
	}
	log.Info("Work directory: ", config.workDir)

	log.Info("Syncing Media Library (images) ...")
	syncImages()

	log.Info("Syncing Banners...")
	syncBanners()

	log.Info("Syncing Persons...")
	syncPersons()

	//log.Info("Syncing Sources...")
	//syncSources()

	log.Info("Switching Directories...")
	if err = switchDirectories(); err != nil {
		log.Fatal("switch directories ", err)
	}

	log.Info("Done")
}

type item struct {
	Id      int               `json:"id"`
	Slug    string            `json:"slug"`
	Title   string            `json:"title"`
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta"`
}

var personsLanguage = regexp.MustCompile("^.+?-([a-z]{2})$")
var bannersLanguage = regexp.MustCompile("^en|he|ru|tr|it|ua|es|de$")
var mediaLibraryRE *regexp.Regexp

func syncImages() {
	var images []string
	var err error

	if err = getItem("images", config.url+"get-images", &images); err != nil {
		log.Fatal("get images ", err)
	}

	for _, image := range images {
		log.Info(image)
		image = mediaLibraryRE.ReplaceAllString(image, "")
		if err = saveImage(image); err != nil {
			log.Fatal("save image", err)
		}
	}
}

func syncPersons() {
	var persons []item
	var err error

	if err = getItem("persons", config.url+"get-persons/all", &persons); err != nil {
		log.Fatal("get persons", err)
	}

	for _, person := range persons {
		log.Info(person.Slug)
		if err = checkSlug4Language(person.Slug, personsLanguage); err != nil {
			log.Fatal(err)
		}
		person.Content = mediaLibraryRE.ReplaceAllString(person.Content, config.assetsImages)
		if err = saveItem(filepath.Join(config.workDir, "persons", person.Slug), person); err != nil {
			log.Fatal("save person", err)
		}
	}
}

func syncSources() {
	for page := 1; ; page++ {
		log.Info("Sources page ", page)

		type sourceT struct {
			Content string `json:"content"`
			Slug    string `json:"xslug"`
		}
		var sources []sourceT
		var err error

		url := fmt.Sprintf("%sget-sources?page=%d", config.url, page)
		if err = getItem("sources", url, &sources); err != nil {
			log.Fatal("get sources", err)
		}

		if len(sources) == 0 {
			break
		}

		for _, source := range sources {
			log.Info(source.Slug)
			if err = checkSlug4Language(source.Slug, personsLanguage); err != nil {
				log.Fatal(err)
			}
			matched, _ := regexp.MatchString(".*html$", source.Slug)
			if matched {
				source.Content = mediaLibraryRE.ReplaceAllString(source.Content, config.assetsImages)
			}
			if err = mkdir(0755, config.workDir, "sources", path.Dir(source.Slug)); err != nil {
				log.Fatal("make images dir", err)
			}
			if err = saveItem(filepath.Join(config.workDir, "sources", source.Slug), source.Content); err != nil {
				log.Fatal("save source", err)
			}
		}
	}
}

func syncBanners() {
	var banners []item
	var err error

	if err = getItem("banners", config.url+"get-banners/all", &banners); err != nil {
		log.Fatal("get banners ", err)
	}

	for _, banner := range banners {
		log.Info(banner.Slug)
		if err = checkSlug4Language(banner.Slug, bannersLanguage); err != nil {
			log.Fatal(err)
		}

		banner.Meta["image"] = mediaLibraryRE.ReplaceAllString(banner.Meta["image"], config.assetsImages)

		if err = saveItem(filepath.Join(config.workDir, "banners", banner.Slug), banner); err != nil {
			log.Fatal("save banner", err)
		}
	}
}

func mkdir(permissions os.FileMode, dirs ...string) (err error) {
	dirname := filepath.Join(dirs...)
	_, err = os.Stat(dirname)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(dirname, permissions); err != nil {
			return errors.Wrapf(err, "os.MkdirAll: %s", dirname)
		}
	} else {
		err = errors.Wrapf(err, "os.Stat: %s", dirname)
	}
	return
}

// prepareDirectories create the required folders in the next active folder
func prepareDirectories() (workDir string, err error) {
	workDir = filepath.Join(config.assets, fmt.Sprint(time.Now().Unix()))

	for _,folder := range []string{"banners", "persons", "sources"} {
		if err = mkdir(0755, workDir, folder); err != nil {
			return "", errors.Wrapf(err, "mkdir %s/%s", workDir, folder)
		}
	}

	return workDir, nil
}

// switchDirectories re-creates the "active" symlink to the fresh workDir just created
func switchDirectories() (err error) {
	var active = filepath.Join(config.assets, "active")
	_, err = os.Lstat(active)
	if err == nil {
		if err = os.Remove(active); err != nil {
			return errors.Wrapf(err, " os.Remove: %s", active)
		}
	}

	//  A relative symlink is used to allow different deployment environments.
	// In docker-compose, we and nginx mount the shared config.assets volume into different paths.
	workDirBase := filepath.Base(config.workDir)
	if err = os.Symlink(workDirBase, active); err != nil {
		return errors.Wrapf(err, "os.Symlink: %s to %s", active, workDirBase)
	}
	return
}

func checkSlug4Language(slug string, pattern *regexp.Regexp) (err error) {
	x := pattern.FindStringSubmatch(slug)
	if len(x) == 0 {
		return errors.Wrapf(err, "\t- slug MUST include language, but does not")
	}

	return
}

func saveItem(path string, v interface{}) (err error) {
	m, err := json.Marshal(v)
	if err != nil {
		return errors.Wrapf(err, "json.Marshal: %s", path)
	}
	err = ioutil.WriteFile(path, m, 0644)
	if err != nil {
		return errors.Wrapf(err, "ioutil.WriteFile: %s", path)
	}
	return
}

func getItem(name string, url string, v interface{}) (err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrapf(err, "http.NewRequest: %s", name)
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "client.Do: %s", name)
	}
	defer func() {
		x := res.Body.Close()
		if x != nil {
			err = errors.Wrapf(x, "res.Body.Close: %s", name)
			log.Fatal(err)
		}
	}()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrapf(err, "ioutil.ReadAll: %s", name)
	}
	err = json.Unmarshal(body, v)
	if err != nil {
		return errors.Wrapf(err, "json.Unmarshal: %s", name)
	}
	return
}

func saveImage(image string) (err error) {
	// create directories for images
	if err = mkdir(0755, config.workDir, "images", path.Dir(image)); err != nil {
		return errors.Wrapf(err,"mkdir %s", image)
	}

	// copy images
	res, err := http.Get(fmt.Sprintf("https://%s%s",config.imageUrl, image))
	if err != nil {
		return errors.Wrapf(err, "http.Get %s", image)
	}
	out, err := os.Create(filepath.Join(config.workDir, "images", image))
	if err != nil {
		return errors.Wrapf(err, "os.Create %s", image)
	}
	// Write the body to file
	if _, err = io.Copy(out, res.Body); err != nil {
		return errors.Wrapf(err, "io.Copy %s", image)
	}
	defer func() {
		x := res.Body.Close()
		if x != nil {
			err = errors.Wrapf(x, "saveImage close body")
			log.Fatal(err)
		}
	}()
	if err = out.Close(); err != nil {
		return errors.Wrapf(err, "out.Close %s", image)
	}
	return
}
