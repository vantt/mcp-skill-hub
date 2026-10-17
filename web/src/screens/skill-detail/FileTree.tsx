import type { RowStatus, TreeRow } from '../../state/changeset';

const STATUS_WORD: Record<Exclude<RowStatus, ''>, string> = {
  A: 'added',
  M: 'modified',
  D: 'deleted',
  R: 'renamed',
};

interface Node {
  name: string;
  dirs: Map<string, Node>;
  files: TreeRow[];
}

function buildTree(rows: TreeRow[]): Node {
  const root: Node = { name: '', dirs: new Map(), files: [] };
  for (const row of rows) {
    const parts = row.path.split('/');
    let node = root;
    for (const part of parts.slice(0, -1)) {
      let next = node.dirs.get(part);
      if (!next) {
        next = { name: part, dirs: new Map(), files: [] };
        node.dirs.set(part, next);
      }
      node = next;
    }
    node.files.push(row);
  }
  return root;
}

const ICON = { width: 16, height: 16, viewBox: '0 0 16 16', fill: 'none', stroke: 'currentColor', strokeWidth: 1.4, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true, focusable: false } as const;

export function FolderIcon({ open = true }: { open?: boolean }) {
  return (
    <svg {...ICON} fill="currentColor" fillOpacity={0.18}>
      <path d={open ? 'M1.5 4.5v8h11l2-6h-11l-2 4.5V3h4l1.5 1.5h5v2' : 'M1.5 3h4l1.5 1.5h7.5v8h-13z'} />
    </svg>
  );
}

export function FileIcon() {
  return (
    <svg {...ICON}>
      <path d="M4 1.5h5l3 3v10H4zM9 1.5v3h3" />
    </svg>
  );
}

export function UploadIcon() {
  return (
    <svg {...ICON}>
      <path d="M8 11V2.5M4.5 6 8 2.5 11.5 6M2.5 13.5h11" />
    </svg>
  );
}

function baseName(path: string): string {
  return path.split('/').pop() || path;
}

interface FileTreeProps {
  rows: TreeRow[];
  selected: string | null;
  onSelect: (path: string) => void;
}

// The files of a skill as a tree: SKILL.md first, then folders, then the other files at the top.
// A file with a pending change carries a one-letter mark (A, M, D, R) and its meaning in words.
export function FileTree({ rows, selected, onSelect }: FileTreeProps) {
  const root = buildTree(rows);

  const renderFile = (row: TreeRow, depth: number) => {
    const word = row.status ? STATUS_WORD[row.status] : '';
    const label = row.from ? `${row.path} (renamed from ${row.from})` : word ? `${row.path} (${word})` : row.path;
    const active = selected === row.path;
    return (
      <li key={row.path} role="none">
        <button
          type="button"
          role="treeitem"
          aria-label={label}
          aria-selected={active}
          className={`app-filetree__file ${active ? 'app-filetree__file--active' : ''}`}
          style={{ paddingLeft: `${8 + depth * 16}px`, textDecoration: row.status === 'D' ? 'line-through' : undefined }}
          onClick={() => onSelect(row.path)}
        >
          <span className="app-filetree__icon" aria-hidden="true">
            <FileIcon />
          </span>
          <span className="app-wrap" style={{ flex: 1, minWidth: 0, textAlign: 'left' }}>
            {baseName(row.path)}
          </span>
          {row.status && (
            <span className={`app-filetree__mark app-filetree__mark--${row.status}`} aria-hidden="true">
              {row.status}
            </span>
          )}
        </button>
      </li>
    );
  };

  const renderNode = (node: Node, depth: number): React.ReactNode[] => {
    const skillMd = node.files.filter((f) => f.path === 'SKILL.md');
    const others = node.files.filter((f) => f.path !== 'SKILL.md').sort((a, b) => a.path.localeCompare(b.path));
    const dirs = [...node.dirs.values()].sort((a, b) => a.name.localeCompare(b.name));
    return [
      ...skillMd.map((f) => renderFile(f, depth)),
      ...dirs.map((dir) => (
        <li key={`dir:${dir.name}:${depth}`} role="none">
          <div className="app-filetree__dir" style={{ paddingLeft: `${8 + depth * 16}px` }}>
            <span className="app-filetree__icon" aria-hidden="true">
              <FolderIcon />
            </span>
            <span>{dir.name}</span>
          </div>
          <ul role="group" className="app-filetree__list">
            {renderNode(dir, depth + 1)}
          </ul>
        </li>
      )),
      ...others.map((f) => renderFile(f, depth)),
    ];
  };

  return (
    <ul role="tree" aria-label="Skill files" className="app-filetree__list">
      {renderNode(root, 0)}
    </ul>
  );
}
