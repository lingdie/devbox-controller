package tag

type Client interface {
	TagImage(hostName string, imageName string, oldTag string, newTag string) error
}
