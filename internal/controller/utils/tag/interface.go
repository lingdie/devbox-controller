package tag

type ReleaseTagClient interface {
	TagImage(username string, password string, repositoryName string, imageName string, oldTag string, newTag string) error
}
