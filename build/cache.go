package build

import (
	"crypto/sha256"
	"fmt"

	log "github.com/Sirupsen/logrus"
)

func (b *Builder) probeCache() bool {
	repo, tag := b.getCachedImageName()
	cachedImage := fmt.Sprintf("%s:%s", repo, tag)

	imgInfo, err := b.client.InspectImage(cachedImage)
	if err != nil {
		log.Debugf("unable to inspect cached image: %s", err)
		return false
	}

	b.imageID = imgInfo.Id
	b.uncommitted = false
	b.uncommittedCommands = nil

	fmt.Fprintf(b.out, " cache hit ---> %s\n", b.imageID)

	return true
}

func (b *Builder) getCachedImageName() (repo, tag string) {
	hasher := sha256.New()
	hasher.Write([]byte(b.imageID))

	for _, command := range b.uncommittedCommands {
		hasher.Write([]byte(command))
	}

	return "_build_cache", fmt.Sprintf("%x", hasher.Sum(nil))
}
