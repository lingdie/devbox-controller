package dockerhub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

type DockerhubClient struct {
	AuthPath     string
	RegistryPath string
}

func (t *DockerhubClient) TagImage(username string, password string, repositoryName string, imageName string, oldTag string, newTag string) error {
	fmt.Println("controller已经成功进入啦！tagimage")
	token, err := t.login(t.AuthPath, username, password, repositoryName, imageName)
	if err != nil {
		return err
	}
	fmt.Println("token: %s\n", token)
	manifest, err := t.pullManifest(t.RegistryPath, token, repositoryName, imageName, oldTag)
	if err != nil {
		return err
	}
	fmt.Println("manifest: %s\n", string(manifest))
	if err := t.pushManifest(t.RegistryPath, token, repositoryName, imageName, newTag, manifest); err != nil {
		fmt.Println(err)
	}
	return nil
}

func (t *DockerhubClient) login(authPath string, username string, password string, repositoryName string, image string) (string, error) {
	var (
		client = http.DefaultClient
		url    = authPath + repositoryName + "/" + image + ":pull,push"
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(resp.Status)
	}

	bodyText, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var data struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		IssuedAt    string `json:"issued_at"`
	}
	if err := json.Unmarshal(bodyText, &data); err != nil {
		return "", err
	}
	if data.Token == "" {
		return "", errors.New("empty token")
	}
	return data.Token, nil
}

func (t *DockerhubClient) pullManifest(registryPath string, token string, repository string, imageName string, tag string) ([]byte, error) {
	var (
		client = http.DefaultClient
		url    = registryPath + repository + "/" + imageName + "/manifests/" + tag
	)
	fmt.Println(url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	bodyText, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return bodyText, nil
}

func (t *DockerhubClient) pushManifest(registryPath string, token string, repository string, imageName string, tag string, manifest []byte) error {
	var (
		client = http.DefaultClient
		url    = registryPath + repository + "/" + imageName + "/manifests/" + tag
	)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(manifest))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-type", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return errors.New(resp.Status)
	}

	return nil
}
