package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/cyverse-de/version"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/cyverse-de/messaging.v2"

	"github.com/cyverse-de/configurate"
	"github.com/cyverse-de/logcabin"
	"gopkg.in/cyverse-de/model.v1"
)

var filenameRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.json$`)

// DockerClient is the subset of the docker client functions that image-janitor
// uses.
type DockerClient interface {
	SafelyRemoveImage(string, string) error
	Images() ([]string, error)
	DanglingImages() ([]string, error)
	SafelyRemoveImageByID(string) error
}

type dclient struct {
	client *docker.Client
}

func (d *dclient) SafelyRemoveImage(name, tag string) error {
	combinedName := fmt.Sprintf("%s:%s", name, tag)
	err := d.client.RemoveImage(combinedName)
	if err != nil {
		err = errors.Wrapf(err, "error safely removing image %s", combinedName)
	}
	return err
}

func (d *dclient) Images() ([]string, error) {
	images, err := d.client.ListImages(docker.ListImagesOptions{All: true})
	if err != nil {
		return nil, err
	}
	var retval []string
	for _, img := range images {
		repos := img.RepoTags
		for _, r := range repos {
			retval = append(retval, r)
		}
	}
	return retval, nil
}

func (d *dclient) DanglingImages() ([]string, error) {
	var err error

	imageFilter := map[string][]string{
		"dangling": []string{"true"},
	}

	images, err := d.client.ListImages(docker.ListImagesOptions{
		Filters: imageFilter,
	})
	if err != nil {
		return nil, err
	}

	var retval []string
	for _, img := range images {
		retval = append(retval, img.ID)
	}

	return retval, nil
}

func (d *dclient) SafelyRemoveImageByID(id string) error {
	err := d.client.RemoveImageExtended(id, docker.RemoveImageOptions{
		Force:   false,
		NoPrune: true,
	})
	if err != nil {
		return err
	}
	return err
}

// Messenger defines an interface for handling AMQP operations. This is the
// subset of functionality needed by job-status-recorder.
type Messenger interface {
	AddConsumer(string, string, string, string, messaging.MessageHandler)
	Close()
	Listen()
	Publish(string, []byte) error
	SetupPublishing(string) error
}

// ImageJanitor contains application state for image-janitor
type ImageJanitor struct {
	cfg    *viper.Viper
	client Messenger
}

// New returns a *ImageJanitor
func New(cfg *viper.Viper) *ImageJanitor {
	return &ImageJanitor{
		cfg: cfg,
	}
}

// jobFiles lists the files in the directory that have a filename matching the
// filenameRegex pattern.
func (i *ImageJanitor) jobFiles(dir string) ([]string, error) {
	var retval []string

	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Mode().IsRegular() {
			if !filenameRegex.Match([]byte(entry.Name())) {
				continue
			}
			retval = append(retval, path.Join(dir, entry.Name()))
		}
	}

	return retval, nil
}

// jobs returns a list of model.Job's that were read from the file paths passed
// in.
func (i *ImageJanitor) jobs(filepaths []string) ([]model.Job, error) {
	var retval []model.Job

	for _, filepath := range filepaths {
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			return retval, err
		}

		job, err := model.NewFromData(i.cfg, data)
		if err != nil {
			logcabin.Error.Printf("error parsing %s, continuing: %s", filepath, err)
			continue
		}

		retval = append(retval, *job)
	}

	return retval, nil
}

// jobImages returns a uniquified list of container images referenced in the
// model.Job's that were passed in.
func (i *ImageJanitor) jobImages(jobs []model.Job) []string {
	unique := make(map[string]bool)

	for _, job := range jobs {
		jobImages := job.ContainerImages()
		for _, ji := range jobImages {
			repoTag := fmt.Sprintf("%s:%s", ji.Name, ji.Tag)
			unique[repoTag] = true
		}
	}

	var retval []string
	for tag := range unique {
		retval = append(retval, tag)
	}

	return retval
}

// removableImages takes in a list of images referred to in the jobs and a list
// of images returned by Docker and returns the ones that can be safely removed.
// Images are considered safe if they're listed in the Docker images but not
// in the job images.
func (i *ImageJanitor) removableImages(jobImages, dockerImages []string) []string {
	imageMap := make(map[string]bool)

	for _, di := range dockerImages {
		imageMap[di] = true
	}

	for _, ji := range jobImages {
		imageMap[ji] = false
	}

	var retval []string
	for img, isRemovable := range imageMap {

		if isRemovable && img != "<none>:<none>" {
			retval = append(retval, img)
		}
	}

	return retval
}

// removeImage uses the dockerops.Docker client to safely remove the specified
// image.
func (i *ImageJanitor) removeImage(client DockerClient, image string) error {
	var (
		err       error
		parts     []string
		name, tag string
	)

	parts = strings.Split(image, ":")
	if len(parts) > 1 {
		name = strings.Join(parts[0:len(parts)-1], ":")
		tag = parts[len(parts)-1]
		if err = client.SafelyRemoveImage(name, tag); err != nil {
			return err
		}
	}

	return err
}

// removeUnusedImages removes all of the images returned by removeImage() from
// the connected Docker Engine.
func (i *ImageJanitor) removeUnusedImages(client DockerClient, readFrom string) {
	listing, err := i.jobFiles(readFrom)
	if err != nil {
		logcabin.Error.Print(err)
		return
	}

	jobList, err := i.jobs(listing)
	if err != nil {
		logcabin.Error.Print(err)
		return
	}

	imagesFromJobs := i.jobImages(jobList)
	imagesFromDocker, err := client.Images()
	if err != nil {
		logcabin.Error.Print(err)
		return
	}

	rmables := i.removableImages(imagesFromJobs, imagesFromDocker)

	excludesPath := path.Join(readFrom, "excludes")
	excludesFile, err := os.Open(excludesPath)
	if err != nil {
		logcabin.Error.Printf("error opening excludes file: %s\n", err)
	}
	defer excludesFile.Close()

	excludes, err := i.readExcludes(excludesFile)
	if err != nil {
		logcabin.Error.Println(err)
	}

	for _, removableImage := range rmables {
		if _, ok := excludes[removableImage]; !ok {
			if err = i.removeImage(client, removableImage); err != nil {
				errmsg := fmt.Sprintf("error removing image %s: %s", removableImage, err)
				logcabin.Error.Println(errmsg)
			}
		}
	}

	danglingImages, err := client.DanglingImages()
	if err != nil {
		logcabin.Error.Println(err)
	}
	for _, di := range danglingImages {
		if err = client.SafelyRemoveImageByID(di); err != nil {
			logcabin.Error.Println(err)
		}
	}
}

func (i *ImageJanitor) readExcludes(readFrom io.Reader) (map[string]bool, error) {
	retval := make(map[string]bool)

	// excludesPath := path.Join(readFrom, "excludes")
	excludesBytes, err := ioutil.ReadAll(readFrom)
	if err != nil {
		return retval, err
	}

	lines := bytes.Split(excludesBytes, []byte("\n"))
	for _, line := range lines {
		if !bytes.Equal(line, []byte("")) {
			retval[string(line)] = true
		}
	}

	return retval, nil
}

func main() {
	var (
		showVersion   = flag.Bool("version", false, "Print version information.")
		interval      = flag.String("interval", "1m", "Time between clean up attempts.")
		cfgPath       = flag.String("config", "/etc/jobservices.yml", "Path to the config.")
		readFrom      = flag.String("read-from", "/opt/image-janitor", "The directory that job files are read from.")
		dockerURI     = flag.String("docker", "unix:///var/run/docker.sock", "The URI for connecting to docker.")
		cfg           *viper.Viper
		err           error
		timerDuration time.Duration
	)

	flag.Parse()

	logcabin.Init("image-janitor", "image-janitor")

	if *showVersion {
		version.AppVersion()
		os.Exit(0)
	}

	r, err := os.Open(*readFrom)
	if err != nil {
		logcabin.Error.Fatal(err)
	}
	r.Close()

	logcabin.Info.Printf("Parsing interval %s", *interval)
	if timerDuration, err = time.ParseDuration(*interval); err != nil {
		logcabin.Error.Fatal(err)
	}
	logcabin.Info.Printf("Successfully parsed interval %s", *interval)

	if *cfgPath == "" {
		logcabin.Error.Fatal("--config must be set.")
	}

	logcabin.Info.Printf("Reading config from %s", *cfgPath)
	if r, err = os.Open(*cfgPath); err != nil {
		logcabin.Error.Fatal(*cfgPath)
	}
	r.Close()

	cfg, err = configurate.InitDefaults(*cfgPath, configurate.JobServicesDefaults)
	if err != nil {
		logcabin.Error.Fatal(err)
	}
	logcabin.Info.Printf("Done reading config from %s", *cfgPath)

	amqpURI := cfg.GetString("amqp.uri")
	exchangeName := cfg.GetString("amqp.exchange.name")

	app := New(cfg)

	app.client, err = messaging.NewClient(amqpURI, false)
	if err != nil {
		logcabin.Error.Fatal(err)
	}
	defer app.client.Close()

	go app.client.Listen()

	app.client.SetupPublishing(exchangeName)

	logcabin.Info.Printf("Connecting to Docker at %s", *dockerURI)
	dc, err := docker.NewClient(*dockerURI)
	if err != nil {
		logcabin.Error.Fatal(err)
	}
	logcabin.Info.Printf("Done connecting to Docker at %s", *dockerURI)

	client := &dclient{dc}

	timer := time.NewTicker(timerDuration)
	for {
		select {
		case <-timer.C:
			app.removeUnusedImages(client, *readFrom)
		}
	}
}
