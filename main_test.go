package main

import (
	"bytes"
	"fmt"
	"testing"
)

type DockerTest struct {
	removedImages   []string
	images          map[string]string
	danglingImages  map[string]string
	removedImageIDs []string
}

func NewDockerTest() *DockerTest {
	return &DockerTest{
		removedImages: make([]string, 0),
		images: map[string]string{
			"1": "alpine:latest",
			"2": "test00:tag00",
			"3": "test01:tag01",
			"4": "test02:tag02",
		},
		danglingImages: map[string]string{
			"5": "d_test00:tag00",
			"6": "d_test01:tag01",
			"7": "d_test02:tag02",
		},
		removedImageIDs: make([]string, 0),
	}
}

func (d *DockerTest) SafelyRemoveImage(name, tag string) error {
	img := fmt.Sprintf("%s:%s", name, tag)
	for k, v := range d.images {
		if v == img {
			delete(d.images, k)
		}
	}
	d.removedImages = append(d.removedImages, fmt.Sprintf("%s:%s", name, tag))
	return nil
}

func (d *DockerTest) Images() ([]string, error) {
	var retval []string
	for _, v := range d.images {
		retval = append(retval, v)
	}
	return retval, nil
}

func (d *DockerTest) DanglingImages() ([]string, error) {
	var retval []string
	for _, v := range d.danglingImages {
		retval = append(retval, v)
	}
	return retval, nil
}

func (d *DockerTest) SafelyRemoveImageByID(id string) error {
	for k := range d.images {
		if k == id {
			delete(d.images, k)
		}
	}
	d.removedImageIDs = append(d.removedImageIDs, id)
	return nil
}

func TestJobFiles(t *testing.T) {
	app := New()
	listing, err := app.jobFiles("test/")
	if err != nil {
		t.Error(err)
	}
	expectedLength := 3
	actualLength := len(listing)
	if actualLength != expectedLength {
		t.Errorf("length of listing was %d instead of %d", actualLength, expectedLength)
	}
	found0 := false
	found1 := false
	found2 := false
	for _, filepath := range listing {
		if filepath == "test/00000000-0000-0000-0000-000000000000.json" {
			found0 = true
		}
		if filepath == "test/00000000-0000-0000-0000-000000000001.json" {
			found1 = true
		}
		if filepath == "test/00000000-0000-0000-0000-000000000002.json" {
			found2 = true
		}
	}
	if !found0 {
		t.Error("Path test/00000000-0000-0000-0000-000000000000.json was not found")
	}
	if !found1 {
		t.Error("Path test/00000000-0000-0000-0000-000000000001.json was not found")
	}
	if !found2 {
		t.Error("Path test/00000000-0000-0000-0000-000000000002.json was not found")
	}
}

func TestJobs(t *testing.T) {
	app := New()
	paths, err := app.jobFiles("test/")
	if err != nil {
		t.Error(err)
	}
	listing, err := app.jobs(paths)
	if err != nil {
		t.Error(err)
	}
	actualLength := len(listing)
	expectedLength := 3
	if actualLength != expectedLength {
		t.Errorf("length of listing was %d instead of %d", actualLength, expectedLength)
	}
	found0 := false
	found1 := false
	found2 := false
	for _, j := range listing {
		switch j.InvocationID {
		case "07b04ce2-7757-4b21-9e15-0b4c2f44be26":
			found0 = true
		case "07b04ce2-7757-4b21-9e15-0b4c2f44be27":
			found1 = true
		case "07b04ce2-7757-4b21-9e15-0b4c2f44be28":
			found2 = true
		}
	}
	if !found0 {
		t.Error("InvocationID 07b04ce2-7757-4b21-9e15-0b4c2f44be26 was not found")
	}
	if !found1 {
		t.Error("InvocationID 07b04ce2-7757-4b21-9e15-0b4c2f44be27 was not found")
	}
	if !found2 {
		t.Error("InvocationID 07b04ce2-7757-4b21-9e15-0b4c2f44be28 was not found")
	}
}

func TestJobImages(t *testing.T) {
	app := New()
	paths, err := app.jobFiles("test/")
	if err != nil {
		t.Error(err)
	}
	listing, err := app.jobs(paths)
	if err != nil {
		t.Error(err)
	}
	images := app.jobImages(listing)
	actualLength := len(images)
	expectedLength := 2
	if actualLength != expectedLength {
		t.Errorf("Number of images was %d instead of %d", actualLength, expectedLength)
	}
	found0 := false
	found1 := false
	for _, i := range images {
		switch i {
		case "gims.iplantcollaborative.org:5000/backwards-compat:latest":
			found0 = true
		case "gims.iplantcollaborative.org:5000/fake-image:latest":
			found1 = true
		}
	}
	if !found0 {
		t.Error("Did not find the backwards-compat image")
	}
	if !found1 {
		t.Error("Did not find the fake-image image")
	}
}

func TestRemovableImages(t *testing.T) {
	app := New()
	paths, err := app.jobFiles("test/")
	if err != nil {
		t.Error(err)
	}
	listing, err := app.jobs(paths)
	if err != nil {
		t.Error(err)
	}
	jImages := app.jobImages(listing)
	dImages := []string{
		"gims.iplantcollaborative.org:5000/backwards-compat:latest",
		"gims.iplantcollaborative.org:5000/fake-image:latest",
		"not-listed",
	}
	rImages := app.removableImages(jImages, dImages)
	actualLength := len(rImages)
	expectedLength := 1
	if actualLength != expectedLength {
		t.Errorf("The number of removable images was %d instead of %d", actualLength, expectedLength)
	}
	actual := rImages[0]
	expected := "not-listed"
	if actual != expected {
		t.Errorf("Removable image was %s instead of %s", actual, expected)
	}
}

func TestReadExcludes(t *testing.T) {
	app := New()
	contents := [][]byte{
		[]byte("line1"),
		[]byte("line1\nline2\n"),
		[]byte("line1\nline2\nline3"),
		[]byte("\nline1\nline2\nline3\n"),
	}
	for _, content := range contents {
		buf := bytes.NewBuffer(content)
		excludes, err := app.readExcludes(buf)
		if err != nil {
			t.Errorf("err parsing excludes: %s", err)
		}
		for e := range excludes {
			if e == "" {
				t.Error("exclusion was an empty string")
			}
		}
	}
}

func TestRemoveImage(t *testing.T) {
	app := New()
	client := NewDockerTest()
	err := app.removeImage(client, "alpine:latest")
	if err != nil {
		t.Error(err)
	}
	images, err := client.Images()
	if err != nil {
		t.Error(err)
	}
	found := false
	for _, i := range images {
		if i == "alpine:latest" {
			found = true
		}
	}
	if found {
		t.Error("alpine:latest was found")
	}
}

func TestRemoveUnusedImages(t *testing.T) {
	app := New()
	client := NewDockerTest()
	app.removeUnusedImages(client, "test/")
	images, err := client.Images()
	if err != nil {
		t.Error(err)
	}
	found := false
	for _, i := range images {
		if i == "alpine:latest" {
			found = true
		}
	}
	if found {
		t.Error("alpine:latest was found")
	}
}
