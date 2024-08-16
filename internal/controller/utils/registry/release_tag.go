package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

type RegistryClient struct {
	Username string
	Password string
}

func (t *RegistryClient) TagImage(hostName string, imageName string, oldTag string, newTag string) error {
	//token, err := t.login(t.AuthPath, username, password, imageName)
	//if err != nil {
	//	return err
	//}
	manifest, err := t.pullManifest(t.Username, t.Password, hostName, imageName, oldTag)
	if err != nil {
		return err
	}
	if err := t.pushManifest(t.Username, t.Password, hostName, imageName, newTag, manifest); err != nil {
		fmt.Println(err)
	}
	return nil
}

func (t *RegistryClient) login(authPath string, username string, password string, imageName string) (string, error) {
	var (
		client = http.DefaultClient
		url    = authPath + imageName + ":pull,push"
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

func (t *RegistryClient) pullManifest(username string, password string, hostName string, imageName string, tag string) ([]byte, error) {
	var (
		client = http.DefaultClient
		url    = "http://" + hostName + "/v2/" + imageName + "/manifests/" + tag
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	fmt.Println("访问的url为：" + url)
	req.SetBasicAuth(username, password)
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

func (t *RegistryClient) pushManifest(username string, password string, hostName string, imageName string, tag string, manifest []byte) error {
	var (
		client = http.DefaultClient
		url    = "http://" + hostName + "/v2/" + imageName + "/manifests/" + tag
	)

	fmt.Println("访问的url为：" + url)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(manifest))
	if err != nil {
		return err
	}

	req.SetBasicAuth(username, password)
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
