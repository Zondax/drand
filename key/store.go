package key

import (
	"fmt"
	"os"
	"path"
	"reflect"

	"github.com/drand/drand/common/constants"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/fs"
)

// Store abstracts the loading and saving of any private/public cryptographic
// material to be used by drand. For the moment, only a file based store is
// implemented.
type Store interface {
	// SaveKeyPair saves the private key generated by drand as well as the
	// public identity key associated
	SaveKeyPair(p *Pair) error
	// LoadKeyPair loads the private/public key pair associated with the drand
	// operator
	LoadKeyPair() (*Pair, error)
	SaveShare(share *Share) error
	LoadShare() (*Share, error)
	SaveGroup(*Group) error
	LoadGroup() (*Group, error)
	Reset(...ResetOption) error
}

// KeyFolderName is the name of the folder where drand keeps its keys
const KeyFolderName = "key"

// GroupFolderName is the name of the folder where drand keeps its group files
const GroupFolderName = "groups"
const keyFileName = "drand_id"
const privateExtension = ".private"
const publicExtension = ".public"
const groupFileName = "drand_group.toml"
const shareFileName = "dist_key.private"
const distKeyFileName = "dist_key.public"

// Tomler represents any struct that can be (un)marshaled into/from toml format
// XXX surely golang reflect package can automatically return the TOMLValue()
// for us
type Tomler interface {
	TOML() interface{}
	FromTOML(i interface{}) error
	TOMLValue() interface{}
}

// fileStore is a Store using filesystem to store informations
type fileStore struct {
	baseFolder     string
	beaconID       string
	privateKeyFile string
	publicKeyFile  string
	shareFile      string
	distKeyFile    string
	groupFile      string
}

// GetFirstStore will return the first store from the stores map
func GetFirstStore(stores map[string]Store) (string, Store) {
	for k, v := range stores {
		return k, v
	}
	return "", nil
}

// NewFileStores will list all folder on base path and load every file store it can find. It will
// return a map with a beacon id as key and a file store as value.
func NewFileStores(baseFolder string) (map[string]Store, error) {
	fileStores := make(map[string]Store)
	fi, err := os.ReadDir(path.Join(baseFolder))
	if err != nil {
		return nil, err
	}

	for _, f := range fi {
		if f.IsDir() {
			fileStores[f.Name()] = NewFileStore(baseFolder, f.Name())
		}
	}

	if len(fileStores) == 0 {
		fileStores[constants.DefaultBeaconID] = NewFileStore(baseFolder, constants.DefaultBeaconID)
	}

	return fileStores, nil
}

// NewFileStore is used to create the config folder and all the subfolders.
// If a folder alredy exists, we simply check the rights
func NewFileStore(baseFolder, beaconID string) Store {
	if beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	store := &fileStore{baseFolder: baseFolder, beaconID: beaconID}

	keyFolder := fs.CreateSecureFolder(path.Join(baseFolder, beaconID, KeyFolderName))
	groupFolder := fs.CreateSecureFolder(path.Join(baseFolder, beaconID, GroupFolderName))

	store.privateKeyFile = path.Join(keyFolder, keyFileName) + privateExtension
	store.publicKeyFile = path.Join(keyFolder, keyFileName) + publicExtension
	store.groupFile = path.Join(groupFolder, groupFileName)
	store.shareFile = path.Join(groupFolder, shareFileName)
	store.distKeyFile = path.Join(groupFolder, distKeyFileName)

	return store
}

// FIXME After merging to master, we should remove this as master will be able
// to handle the new files structure. (created only for regression test)
// deprecated
// OldNewFileStore is used to create the config folder and all the subfolders in an old way.
// If a folder alredy exists, we simply check the rights
func OldNewFileStore(baseFolder string) Store {
	store := &fileStore{baseFolder: baseFolder}

	keyFolder := fs.CreateSecureFolder(path.Join(baseFolder, KeyFolderName))
	groupFolder := fs.CreateSecureFolder(path.Join(baseFolder, GroupFolderName))

	store.privateKeyFile = path.Join(keyFolder, keyFileName) + privateExtension
	store.publicKeyFile = path.Join(keyFolder, keyFileName) + publicExtension
	store.groupFile = path.Join(groupFolder, groupFileName)
	store.shareFile = path.Join(groupFolder, shareFileName)
	store.distKeyFile = path.Join(groupFolder, distKeyFileName)
	return store
}

// SaveKeyPair first saves the private key in a file with tight permissions and then
// saves the public part in another file.
func (f *fileStore) SaveKeyPair(p *Pair) error {
	if err := Save(f.privateKeyFile, p, true); err != nil {
		return err
	}
	fmt.Printf("Saved the key : %s at %s\n", p.Public.Addr, f.publicKeyFile)
	return Save(f.publicKeyFile, p.Public, false)
}

// LoadKeyPair decode private key first then public
func (f *fileStore) LoadKeyPair() (*Pair, error) {
	p := new(Pair)
	if err := Load(f.privateKeyFile, p); err != nil {
		return nil, err
	}
	return p, Load(f.publicKeyFile, p.Public)
}

func (f *fileStore) LoadGroup() (*Group, error) {
	g := new(Group)
	return g, Load(f.groupFile, g)
}

func (f *fileStore) SaveGroup(g *Group) error {
	return Save(f.groupFile, g, false)
}

func (f *fileStore) SaveShare(share *Share) error {
	fmt.Printf("crypto store: saving private share in %s\n", f.shareFile)
	return Save(f.shareFile, share, true)
}

func (f *fileStore) LoadShare() (*Share, error) {
	s := new(Share)
	return s, Load(f.shareFile, s)
}

func (f *fileStore) Reset(...ResetOption) error {
	if err := Delete(f.distKeyFile); err != nil {
		return fmt.Errorf("drand: err deleting dist. key file: %v", err)
	}
	if err := Delete(f.shareFile); err != nil {
		return fmt.Errorf("drand: errd eleting share file: %v", err)
	}

	if err := Delete(f.groupFile); err != nil {
		return fmt.Errorf("drand: err deleting group file: %v", err)
	}
	return nil
}

// Save the given Tomler interface to the given path. If secure is true, the
// file will have a 0700 security.
// TODO: move that to fs/
func Save(filePath string, t Tomler, secure bool) error {
	var fd *os.File
	var err error
	if secure {
		fd, err = fs.CreateSecureFile(filePath)
	} else {
		fd, err = os.Create(filePath)
	}
	if err != nil {
		return fmt.Errorf("config: can't save %s to %s: %s", reflect.TypeOf(t).String(), filePath, err)
	}
	defer fd.Close()
	return toml.NewEncoder(fd).Encode(t.TOML())
}

// Load the given Tomler from the given file path.
func Load(filePath string, t Tomler) error {
	tomlValue := t.TOMLValue()
	var err error
	if _, err = toml.DecodeFile(filePath, tomlValue); err != nil {
		return err
	}
	return t.FromTOML(tomlValue)
}

// Delete the resource denoted by the given path. If it is a file, it deletes
// the file; if it is a folder it delete the folder and all its content.
func Delete(filePath string) error {
	return os.RemoveAll(filePath)
}

// ResetOption is an option to allow for fine-grained reset
// operations
// XXX TODO
type ResetOption int
