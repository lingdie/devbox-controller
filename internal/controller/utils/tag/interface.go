package tag

type ReleaseTagClient interface {
	TagImage(username string, password string, hostName string, imageName string, oldTag string, newTag string) error
}
