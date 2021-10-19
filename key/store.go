package key

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"

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

//
const DefaultStoreID = "default"

const BeaconsFolderName = "beacons"

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
	groupID        string
	privateKeyFile string
	publicKeyFile  string
	shareFile      string
	distKeyFile    string
	groupFile      string
}

func MigrateOldFolderStructure(baseFolder string) {
	// config folder
	if fs.CreateSecureFolder(baseFolder) == "" {
		fmt.Println("Something went wrong with the config folder. Make sure that you have the appropriate rights.")
		os.Exit(1)
	}

	if fs.CreateSecureFolder(path.Join(baseFolder, BeaconsFolderName)) == "" {
		fmt.Println("Something went wrong with the config folder. Make sure that you have the appropriate rights.")
		os.Exit(1)
	}

	isRequired, err := CheckOldFolderStructMigration(baseFolder)
	if err != nil {
		fmt.Println("Something went wrong with the config folder. Make sure that you have the appropriate rights.")
		os.Exit(1)
	}

	if isRequired {
		if fs.CreateSecureFolder(path.Join(baseFolder, BeaconsFolderName, DefaultStoreID, GroupFolderName)) == "" {
			fmt.Println("Something went wrong with the config folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}

		if err := fs.MoveFolder(path.Join(baseFolder, GroupFolderName),
			path.Join(baseFolder, BeaconsFolderName, DefaultStoreID, GroupFolderName)); err != nil {
			fmt.Println("Something went wrong with the config folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}
}

func CheckOldFolderStructMigration(baseFolder string) (bool, error) {
	folders, err := fs.Folders(baseFolder)
	if err != nil {
		return false, err
	}

	found := false
	groupFolderPath := path.Join(baseFolder, GroupFolderName)

	for _, folderPath := range folders {
		if groupFolderPath == folderPath {
			found = true
			break
		}
	}

	return found, nil
}

func GetFirstStore(stores map[string]Store) (string, Store) {
	for k, v := range stores {
		return k, v
	}
	return "", nil
}

// NewFileStores
func NewFileStores(baseFolder string) map[string]Store {
	MigrateOldFolderStructure(baseFolder)

	fileStores := make(map[string]Store)
	fi, err := ioutil.ReadDir(path.Join(baseFolder, BeaconsFolderName))
	if err != nil {
		return fileStores
	}

	for _, f := range fi {
		if f.IsDir() {
			fileStores[f.Name()] = NewFileStore(baseFolder, f.Name())
		}
	}

	if len(fileStores) == 0 {
		fileStores[DefaultStoreID] = NewFileStore(baseFolder, DefaultStoreID)
	}

	return fileStores
}

// NewFileStore is used to create the config folder and all the subfolders.
// If a folder alredy exists, we simply check the rights
func NewFileStore(baseFolder, groupID string) Store {
	store := &fileStore{baseFolder: baseFolder, groupID: groupID}

	keyFolder := fs.CreateSecureFolder(path.Join(baseFolder, KeyFolderName))
	groupFolder := fs.CreateSecureFolder(path.Join(baseFolder, BeaconsFolderName, groupID, GroupFolderName))

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
