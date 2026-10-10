package hostintegration

import (
	"bytes"
	"os"
)

const curatorReceiptSuffix = ".skillhub-sha256"

func prepareNativeCurator(host Host, path, root string, bundled []byte) (preparedFile, error) {
	if !HostSupportsSkillsExtension(host) {
		return prepareExactFile(ChangeNativeSkill, path, root, bundled)
	}
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	conflict := ""
	if exists {
		owned, err := nativeCuratorOwned(path, root, raw)
		if err != nil {
			return preparedFile{}, err
		}
		if !owned {
			conflict = "native curator differs from the copy installed by skillhub; leave it in place and review manually"
		}
	}
	file := newPreparedFile(host, ChangeNativeSkill, path, raw, nil, mode, exists, conflict)
	file.state.Current = !exists && conflict == ""
	file.preview = "remove previously installed native curator; verified client uses MCP skills extension"
	return file, nil
}

func nativeCuratorOwned(path, root string, raw []byte) (bool, error) {
	receipt, _, exists, err := readManagedFile(path+curatorReceiptSuffix, root)
	if err != nil {
		return false, err
	}
	if exists {
		return string(receipt) == digest(raw, true), nil
	}
	// Older integrations did not write receipts. Only exact bundled bytes are
	// recognizable; other versions and edits require manual review.
	return bytes.Equal(raw, []byte(systemCuratorInstructions())), nil
}

func nativeRemoval(change Change) bool {
	return change.Kind == ChangeNativeSkill && change.Desired == nil && HostSupportsSkillsExtension(change.Host)
}

func writeCuratorReceipt(root *os.Root, writeRoot string, change Change) error {
	path := change.Path + curatorReceiptSuffix
	if _, _, _, err := readManagedFile(path, writeRoot); err != nil {
		return err
	}
	if nativeRemoval(change) {
		relative, err := managedRelativePath(writeRoot, path)
		if err != nil {
			return err
		}
		if err := root.Remove(relative); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return atomicWrite(root, writeRoot, path, []byte(digest(change.Desired, true)), 0o644)
}
