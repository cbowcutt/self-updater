package selfUpdater

type GithubSelfUpdater struct {
}

func (g *GithubSelfUpdater) IsNewerVersionAvailable() (bool, error) {
	return true, nil
}

func (g *GithubSelfUpdater) CheckAndDownloadLatestVersion() error {
	return nil
}
