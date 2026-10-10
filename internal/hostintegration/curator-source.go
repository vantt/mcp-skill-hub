package hostintegration

import "bytes"

const nativeCuratorConflict = "native curator differs from the bundled copy; leave it in place and review manually. `skillhub doctor --fix` restores the bundled copy, after which `skillhub connect` can remove it"

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
		if !nativeCuratorOwned(raw) {
			conflict = nativeCuratorConflict
		}
	}
	file := newPreparedFile(host, ChangeNativeSkill, path, raw, nil, mode, exists, conflict)
	file.state.Current = !exists && conflict == ""
	file.preview = "remove previously installed native curator; verified client uses MCP skills extension"
	return file, nil
}

func nativeCuratorOwned(raw []byte) bool {
	return bytes.Equal(raw, []byte(systemCuratorInstructions()))
}

func nativeRemoval(change Change) bool {
	return change.Kind == ChangeNativeSkill && change.Desired == nil && HostSupportsSkillsExtension(change.Host)
}
