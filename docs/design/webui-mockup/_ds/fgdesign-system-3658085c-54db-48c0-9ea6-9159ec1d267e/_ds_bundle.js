/* @ds-bundle: {"format":4,"namespace":"FgDesignSystem_365808","components":[{"name":"Button","sourcePath":"components/actions/Button.jsx"},{"name":"Profile","sourcePath":"components/crm/Profile.jsx"},{"name":"Facts","sourcePath":"components/crm/Profile.jsx"},{"name":"Pipeline","sourcePath":"components/crm/Profile.jsx"},{"name":"Table","sourcePath":"components/data/Table.jsx"},{"name":"Avatar","sourcePath":"components/display/Avatar.jsx"},{"name":"AvatarCluster","sourcePath":"components/display/AvatarCluster.jsx"},{"name":"Badge","sourcePath":"components/display/Badge.jsx"},{"name":"Card","sourcePath":"components/display/Card.jsx"},{"name":"Chip","sourcePath":"components/display/Chip.jsx"},{"name":"Kpi","sourcePath":"components/display/Kpi.jsx"},{"name":"Status","sourcePath":"components/display/Status.jsx"},{"name":"Banner","sourcePath":"components/feedback/Banner.jsx"},{"name":"Caveat","sourcePath":"components/feedback/Caveat.jsx"},{"name":"Ledger","sourcePath":"components/financial/Ledger.jsx"},{"name":"Field","sourcePath":"components/forms/Field.jsx"},{"name":"Input","sourcePath":"components/forms/Input.jsx"},{"name":"Select","sourcePath":"components/forms/Select.jsx"},{"name":"Switch","sourcePath":"components/forms/Switch.jsx"},{"name":"NavItem","sourcePath":"components/navigation/NavRail.jsx"},{"name":"NavRail","sourcePath":"components/navigation/NavRail.jsx"},{"name":"Tabs","sourcePath":"components/navigation/Tabs.jsx"},{"name":"Tab","sourcePath":"components/navigation/Tabs.jsx"},{"name":"TaskCard","sourcePath":"components/task/TaskCard.jsx"},{"name":"KanbanColumn","sourcePath":"components/task/TaskCard.jsx"},{"name":"Kanban","sourcePath":"components/task/TaskCard.jsx"}],"sourceHashes":{"components/actions/Button.jsx":"21695e620377","components/crm/Profile.jsx":"25a3a958ddca","components/data/Table.jsx":"98338d0e13c2","components/display/Avatar.jsx":"ab95b475f4d3","components/display/AvatarCluster.jsx":"3c9296a73c6c","components/display/Badge.jsx":"b0d449414fc8","components/display/Card.jsx":"e23aec8228a5","components/display/Chip.jsx":"e2e0ff4781ca","components/display/Kpi.jsx":"629148b1d23f","components/display/Status.jsx":"97050373b50d","components/feedback/Banner.jsx":"3487d326af70","components/feedback/Caveat.jsx":"ece536433cf5","components/financial/Ledger.jsx":"2edf993c7b0d","components/forms/Field.jsx":"94bf9ce63f8b","components/forms/Input.jsx":"3806eca9677a","components/forms/Select.jsx":"4c42d2ba93de","components/forms/Switch.jsx":"6cfae3d946f8","components/navigation/NavRail.jsx":"c469ac78e1fd","components/navigation/Tabs.jsx":"cfa7a873ba32","components/task/TaskCard.jsx":"f761e3f94e3b","contract/elements.js":"e1fd975b3bdb","demo/theme-switcher.js":"cd01bd4dddd7"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.FgDesignSystem_365808 = window.FgDesignSystem_365808 || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// components/actions/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-btn — emits the exact contract markup; character comes from --btn-* tokens. */
function Button({
  variant = "primary",
  type = "button",
  className,
  children,
  ...rest
}) {
  return /*#__PURE__*/React.createElement("button", _extends({
    type: type,
    className: cx("fg-btn", "fg-btn--" + variant, className)
  }, rest), children);
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/actions/Button.jsx", error: String((e && e.message) || e) }); }

// components/crm/Profile.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-profile — record header: initials avatar, name, dot-separated meta. */
function Profile({
  name,
  initials,
  meta = [],
  className
}) {
  const parts = [];
  meta.forEach((m, i) => {
    if (i) parts.push(/*#__PURE__*/React.createElement("span", {
      key: "s" + i
    }, "\xB7"));
    parts.push(/*#__PURE__*/React.createElement("span", {
      key: i
    }, m));
  });
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-profile", className)
  }, /*#__PURE__*/React.createElement("div", {
    className: "fg-profile__avatar"
  }, initials), /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    className: "fg-profile__name"
  }, name), parts.length > 0 && /*#__PURE__*/React.createElement("div", {
    className: "fg-profile__meta"
  }, parts)));
}

/** .fg-facts — label/value fact grid; money values get .fg-money. */
function Facts({
  items = [],
  className
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-facts", className)
  }, items.map((f, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    className: "fg-fact"
  }, /*#__PURE__*/React.createElement("div", {
    className: "fg-fact__label"
  }, f.label), /*#__PURE__*/React.createElement("div", {
    className: cx("fg-fact__value", f.money && "fg-money")
  }, f.value))));
}

/** .fg-pipeline — stage stepper; stages before `current` are done. */
function Pipeline({
  stages = [],
  current = 0,
  className
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-pipeline", className)
  }, stages.map((s, i) => /*#__PURE__*/React.createElement("span", {
    key: i,
    className: cx("fg-pipeline__stage", i < current && "fg-pipeline__stage--done", i === current && "fg-pipeline__stage--on")
  }, s)));
}
Object.assign(__ds_scope, { Profile, Facts, Pipeline });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/crm/Profile.jsx", error: String((e && e.message) || e) }); }

// components/data/Table.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}
const box = {
  width: 16,
  height: 16,
  cursor: "pointer"
};

/** .fg-table — sortable heads, row selection, optional bulk bar, optional card frame. */
function Table({
  columns = [],
  rows = [],
  rowKey = "id",
  selectable,
  selected,
  defaultSelected,
  onSelectionChange,
  sort,
  defaultSort,
  onSortChange,
  clickable,
  onRowClick,
  bulkActions,
  framed = true,
  className
}) {
  const [selIn, setSelIn] = React.useState(defaultSelected || []);
  const [sortIn, setSortIn] = React.useState(defaultSort || null);
  const sel = selected ?? selIn;
  const so = sort ?? sortIn;
  const setSel = v => {
    if (selected == null) setSelIn(v);
    onSelectionChange && onSelectionChange(v);
  };
  const setSort = v => {
    if (sort == null) setSortIn(v);
    onSortChange && onSortChange(v);
  };
  const keyOf = (r, i) => (typeof rowKey === "function" ? rowKey(r) : r[rowKey]) ?? i;
  let shown = rows;
  if (so && so.key) {
    const col = columns.find(c => c.key === so.key) || {};
    const val = r => col.sortValue ? col.sortValue(r) : r[so.key];
    shown = [...rows].sort((a, b) => {
      const x = val(a),
        y = val(b);
      const d = typeof x === "number" && typeof y === "number" ? x - y : String(x).localeCompare(String(y));
      return so.dir === "desc" ? -d : d;
    });
  }
  const keys = rows.map(keyOf);
  const all = keys.length > 0 && keys.every(k => sel.includes(k));
  const toggle = k => setSel(sel.includes(k) ? sel.filter(x => x !== k) : [...sel, k]);
  const head = c => {
    if (!c.sortable) return c.label;
    const on = so && so.key === c.key;
    return /*#__PURE__*/React.createElement("button", {
      type: "button",
      className: cx("fg-th-sort", on && (so.dir === "desc" ? "fg-th-sort--desc" : "fg-th-sort--asc")),
      onClick: () => setSort({
        key: c.key,
        dir: on && so.dir === "asc" ? "desc" : "asc"
      })
    }, c.label, /*#__PURE__*/React.createElement("span", {
      className: "fg-th-sort__caret"
    }, on && so.dir === "desc" ? "▼" : "▲"));
  };
  const table = /*#__PURE__*/React.createElement("table", {
    className: cx("fg-table", clickable && "fg-table--clickable", className)
  }, /*#__PURE__*/React.createElement("thead", null, /*#__PURE__*/React.createElement("tr", null, selectable && /*#__PURE__*/React.createElement("th", {
    className: "fg-table__checkcol"
  }, /*#__PURE__*/React.createElement("span", {
    role: "checkbox",
    "aria-checked": all,
    className: cx("fg-check__box", all && "fg-check__box--on"),
    style: box,
    onClick: () => setSel(all ? [] : keys)
  }, all ? "✓" : "")), columns.map(c => /*#__PURE__*/React.createElement("th", {
    key: c.key,
    className: c.num ? "fg-table__num" : undefined
  }, head(c))))), /*#__PURE__*/React.createElement("tbody", null, shown.map((r, i) => {
    const k = keyOf(r, i);
    const on = sel.includes(k);
    return /*#__PURE__*/React.createElement("tr", {
      key: k,
      className: on ? "fg-row--selected" : undefined,
      onClick: onRowClick ? () => onRowClick(r) : undefined
    }, selectable && /*#__PURE__*/React.createElement("td", null, /*#__PURE__*/React.createElement("span", {
      role: "checkbox",
      "aria-checked": on,
      className: cx("fg-check__box", on && "fg-check__box--on"),
      style: box,
      onClick: e => {
        e.stopPropagation();
        toggle(k);
      }
    }, on ? "✓" : "")), columns.map(c => /*#__PURE__*/React.createElement("td", {
      key: c.key,
      className: c.num ? "fg-table__num" : undefined
    }, c.render ? c.render(r) : r[c.key])));
  })));
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      flexDirection: "column",
      gap: "var(--space-3)"
    }
  }, bulkActions && sel.length > 0 && /*#__PURE__*/React.createElement("div", {
    className: "fg-bulkbar"
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-bulkbar__count",
    style: {
      whiteSpace: "nowrap",
      flex: "none"
    }
  }, sel.length, " selected"), /*#__PURE__*/React.createElement("div", {
    className: "fg-bulkbar__actions"
  }, bulkActions), /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: "fg-btn fg-btn--ghost fg-bulkbar__clear",
    onClick: () => setSel([])
  }, "Clear")), framed ? /*#__PURE__*/React.createElement("div", {
    className: "fg-card",
    style: {
      padding: 0,
      overflow: "hidden"
    }
  }, table) : table);
}
Object.assign(__ds_scope, { Table });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/Table.jsx", error: String((e && e.message) || e) }); }

// components/display/Avatar.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-avatar — initials avatar; more = "+N" overflow tail. */
function Avatar({
  more,
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("span", {
    className: cx("fg-avatar", more && "fg-avatar--more", className)
  }, children);
}
Object.assign(__ds_scope, { Avatar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/Avatar.jsx", error: String((e && e.message) || e) }); }

// components/display/AvatarCluster.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-avatar-cluster — overlapping avatar group (multi-assignee). */
function AvatarCluster({
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("span", {
    className: cx("fg-avatar-cluster", className)
  }, children);
}
Object.assign(__ds_scope, { AvatarCluster });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/AvatarCluster.jsx", error: String((e && e.message) || e) }); }

// components/display/Badge.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-badge — small count/label badge. */
function Badge({
  accent,
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("span", {
    className: cx("fg-badge", accent && "fg-badge--accent", className)
  }, children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/Badge.jsx", error: String((e && e.message) || e) }); }

// components/display/Card.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-card — surface with optional head (title + aside), sub, body, foot. */
function Card({
  title,
  aside,
  sub,
  rule,
  sunken,
  foot,
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-card", rule && "fg-card--rule", sunken && "fg-card--sunken", className)
  }, (title != null || aside != null) && /*#__PURE__*/React.createElement("div", {
    className: "fg-card__head"
  }, title != null && /*#__PURE__*/React.createElement("span", {
    className: "fg-card__title"
  }, title), aside), sub != null && /*#__PURE__*/React.createElement("p", {
    className: "fg-card__sub t-body-sm"
  }, sub), children != null && /*#__PURE__*/React.createElement("div", {
    className: "fg-card__body"
  }, children), foot != null && /*#__PURE__*/React.createElement("div", {
    className: "fg-card__foot"
  }, foot));
}
Object.assign(__ds_scope, { Card });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/Card.jsx", error: String((e && e.message) || e) }); }

// components/display/Chip.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-chip — static tone chip, or an interactive filter facet when toggle. */
function Chip({
  tone = "neutral",
  toggle,
  on,
  defaultOn = false,
  onChange,
  className,
  children
}) {
  const [inner, setInner] = React.useState(defaultOn);
  if (!toggle) return /*#__PURE__*/React.createElement("span", {
    className: cx("fg-chip", "fg-chip--" + tone, className)
  }, children);
  const isOn = on ?? inner;
  const flip = () => {
    const n = !isOn;
    if (on === undefined) setInner(n);
    onChange && onChange(n);
  };
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-pressed": isOn,
    className: cx("fg-chip", "fg-chip--toggle", isOn && "fg-chip--on", className),
    onClick: flip
  }, children);
}
Object.assign(__ds_scope, { Chip });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/Chip.jsx", error: String((e && e.message) || e) }); }

// components/display/Kpi.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-kpi — label / value / optional delta foot. Value rides --num-font. */
function Kpi({
  label,
  value,
  delta,
  deltaDir = "flat",
  foot,
  className
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-kpi", className)
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-kpi__label t-label"
  }, label), /*#__PURE__*/React.createElement("span", {
    className: "fg-kpi__value"
  }, value), (delta != null || foot) && /*#__PURE__*/React.createElement("span", {
    className: "fg-kpi__foot"
  }, delta != null && /*#__PURE__*/React.createElement("span", {
    className: "fg-delta fg-delta--" + deltaDir
  }, delta), foot));
}
Object.assign(__ds_scope, { Kpi });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/Kpi.jsx", error: String((e && e.message) || e) }); }

// components/display/Status.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-status — workflow-readiness pill with glowing dot. */
function Status({
  state = "ready",
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("span", {
    className: cx("fg-status", "fg-status--" + state, className)
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-status__dot"
  }), children);
}
Object.assign(__ds_scope, { Status });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/display/Status.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Banner.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-banner — persistent page-level notice with dot, body, optional action link. */
function Banner({
  tone = "info",
  actionLabel,
  actionHref = "#",
  onAction,
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-banner", "fg-banner--" + tone, className)
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-banner__dot"
  }), /*#__PURE__*/React.createElement("span", {
    className: "fg-banner__body"
  }, children), actionLabel && /*#__PURE__*/React.createElement("a", {
    className: "fg-banner__act",
    href: actionHref,
    onClick: onAction
  }, actionLabel));
}
Object.assign(__ds_scope, { Banner });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Banner.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Caveat.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-caveat — block-level callout / attention aside. */
function Caveat({
  tone = "info",
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-caveat", "fg-caveat--" + tone, className)
  }, children);
}
Object.assign(__ds_scope, { Caveat });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Caveat.jsx", error: String((e && e.message) || e) }); }

// components/financial/Ledger.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}
const money = t => cx("fg-ledger__num", t && "fg-money", t === "pos" && "fg-money--pos", t === "neg" && "fg-money--neg");

/** .fg-ledger — financial line-item table with subtotal and total rows; amounts direction-colored. */
function Ledger({
  head = ["Line", "Amount"],
  rows = [],
  total,
  className
}) {
  return /*#__PURE__*/React.createElement("table", {
    className: cx("fg-ledger", className)
  }, head && /*#__PURE__*/React.createElement("thead", null, /*#__PURE__*/React.createElement("tr", null, /*#__PURE__*/React.createElement("th", null, head[0]), /*#__PURE__*/React.createElement("th", {
    className: "fg-ledger__num"
  }, head[1]))), /*#__PURE__*/React.createElement("tbody", null, rows.map((r, i) => /*#__PURE__*/React.createElement("tr", {
    key: i,
    className: r.subtotal ? "fg-ledger__subtotal" : undefined
  }, /*#__PURE__*/React.createElement("td", null, r.label), /*#__PURE__*/React.createElement("td", {
    className: money(r.tone)
  }, r.amount)))), total && /*#__PURE__*/React.createElement("tfoot", null, /*#__PURE__*/React.createElement("tr", null, /*#__PURE__*/React.createElement("td", null, total.label), /*#__PURE__*/React.createElement("td", {
    className: money(total.tone)
  }, total.amount))));
}
Object.assign(__ds_scope, { Ledger });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/financial/Ledger.jsx", error: String((e && e.message) || e) }); }

// components/forms/Field.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-field — label + control + hint/error. Pass an <Input>/<Select> as children. */
function Field({
  label,
  required,
  hint,
  error,
  htmlFor,
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-field", className)
  }, label != null && /*#__PURE__*/React.createElement("label", {
    className: "fg-field__label t-label",
    htmlFor: htmlFor
  }, label, required && /*#__PURE__*/React.createElement(React.Fragment, null, " ", /*#__PURE__*/React.createElement("span", {
    className: "fg-field__req"
  }, "*"))), children, error ? /*#__PURE__*/React.createElement("span", {
    className: "fg-field__error"
  }, error) : hint ? /*#__PURE__*/React.createElement("span", {
    className: "fg-field__hint"
  }, hint) : null);
}
Object.assign(__ds_scope, { Field });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Field.jsx", error: String((e && e.message) || e) }); }

// components/forms/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-input — text input, or textarea when area. */
function Input({
  area,
  mono,
  className,
  ...rest
}) {
  const cls = cx("fg-input", area && "fg-input--area", mono && "fg-input--mono", className);
  return area ? /*#__PURE__*/React.createElement("textarea", _extends({
    className: cls
  }, rest)) : /*#__PURE__*/React.createElement("input", _extends({
    className: cls
  }, rest));
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Input.jsx", error: String((e && e.message) || e) }); }

// components/forms/Select.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-select — native select + chevron part. options: string[] | {value,label}[]. */
function Select({
  options = [],
  className,
  children,
  ...rest
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-select", className)
  }, /*#__PURE__*/React.createElement("select", rest, children || options.map(o => {
    const v = typeof o === "string" ? o : o.value;
    const l = typeof o === "string" ? o : o.label;
    return /*#__PURE__*/React.createElement("option", {
      key: v,
      value: v
    }, l);
  })), /*#__PURE__*/React.createElement("span", {
    className: "fg-select__chev"
  }, "\u25BE"));
}
Object.assign(__ds_scope, { Select });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Select.jsx", error: String((e && e.message) || e) }); }

// components/forms/Switch.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-switch — binary toggle rendered as a <button role="switch">. Controlled (on) or uncontrolled (defaultOn). */
function Switch({
  on,
  defaultOn = false,
  onChange,
  label,
  className,
  disabled
}) {
  const [inner, setInner] = React.useState(defaultOn);
  const isOn = on ?? inner;
  const toggle = () => {
    if (disabled) return;
    const n = !isOn;
    if (on === undefined) setInner(n);
    onChange && onChange(n);
  };
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    role: "switch",
    "aria-checked": isOn,
    disabled: disabled,
    className: cx("fg-switch", isOn && "fg-switch--on", className),
    onClick: toggle,
    style: {
      border: 0,
      background: "none",
      padding: 0
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-switch__track"
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-switch__dot"
  })), label != null && /*#__PURE__*/React.createElement("span", {
    className: "fg-switch__label"
  }, label));
}
Object.assign(__ds_scope, { Switch });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Switch.jsx", error: String((e && e.message) || e) }); }

// components/navigation/NavRail.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-nav__item — one rail entry (icon glyph, label, optional badge). */
function NavItem({
  on,
  icon,
  badge,
  className,
  children,
  ...rest
}) {
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    className: cx("fg-nav__item", on && "fg-nav__item--on", className),
    "aria-current": on ? "page" : undefined
  }, rest), icon != null && /*#__PURE__*/React.createElement("span", {
    className: "fg-nav__icon"
  }, icon), /*#__PURE__*/React.createElement("span", {
    className: "fg-nav__label"
  }, children), badge != null && /*#__PURE__*/React.createElement("span", {
    className: "fg-nav__badge"
  }, badge));
}

/** .fg-nav — vertical navigation rail with optional eyebrow. Controlled (value) or uncontrolled (defaultValue). */
function NavRail({
  eyebrow,
  items,
  value,
  defaultValue,
  onChange,
  className,
  children
}) {
  const first = items && items[0] ? items[0].value : undefined;
  const [inner, setInner] = React.useState(defaultValue ?? first);
  const cur = value ?? inner;
  const pick = v => {
    if (value == null) setInner(v);
    onChange && onChange(v);
  };
  return /*#__PURE__*/React.createElement("nav", {
    className: cx("fg-nav", className)
  }, eyebrow != null && /*#__PURE__*/React.createElement("span", {
    className: "fg-nav__eyebrow t-label"
  }, eyebrow), children ?? (items || []).map(it => /*#__PURE__*/React.createElement(NavItem, {
    key: it.value,
    on: it.value === cur,
    icon: it.icon,
    badge: it.badge,
    onClick: () => pick(it.value)
  }, it.label)));
}
Object.assign(__ds_scope, { NavItem, NavRail });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/NavRail.jsx", error: String((e && e.message) || e) }); }

// components/navigation/Tabs.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-tabs — tab bar. Pass items (string[] | {value,label}[]) or raw <Tab> children. */
function Tabs({
  items,
  value,
  defaultValue,
  onChange,
  className,
  children
}) {
  const norm = (items || []).map(i => typeof i === "string" ? {
    value: i,
    label: i
  } : i);
  const [inner, setInner] = React.useState(defaultValue ?? (norm[0] && norm[0].value));
  const cur = value ?? inner;
  const pick = v => {
    if (value === undefined) setInner(v);
    onChange && onChange(v);
  };
  return /*#__PURE__*/React.createElement("div", {
    role: "tablist",
    className: cx("fg-tabs", className)
  }, children || norm.map(i => /*#__PURE__*/React.createElement(Tab, {
    key: i.value,
    on: i.value === cur,
    onClick: () => pick(i.value)
  }, i.label)));
}
/** .fg-tab — single tab. */
function Tab({
  on,
  className,
  children,
  ...rest
}) {
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    role: "tab",
    "aria-selected": !!on,
    className: cx("fg-tab", on && "fg-tab--on", className)
  }, rest), children);
}
Object.assign(__ds_scope, { Tabs, Tab });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/Tabs.jsx", error: String((e && e.message) || e) }); }

// components/task/TaskCard.jsx
try { (() => {
function cx(...a) {
  return a.filter(Boolean).join(" ");
}

/** .fg-task — kanban / list task card. */
function TaskCard({
  title,
  tags = [],
  priority,
  progress,
  assignee,
  due,
  overdue,
  className,
  children
}) {
  const hasFoot = assignee || due;
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-task", className)
  }, title != null && /*#__PURE__*/React.createElement("div", {
    className: "fg-task__title"
  }, title), (tags.length > 0 || priority) && /*#__PURE__*/React.createElement("div", {
    className: "fg-task__meta"
  }, tags.map((t, i) => /*#__PURE__*/React.createElement("span", {
    key: i,
    className: cx("fg-tag", "fg-tag--" + ((typeof t === "string" ? null : t.tone) || i % 5 + 1))
  }, typeof t === "string" ? t : t.label)), priority && /*#__PURE__*/React.createElement("span", {
    className: "fg-priority fg-priority--" + priority
  }, priority)), progress != null && /*#__PURE__*/React.createElement("div", {
    className: "fg-progress"
  }, /*#__PURE__*/React.createElement("div", {
    className: "fg-progress__bar",
    style: {
      width: progress + "%"
    }
  })), children, hasFoot && /*#__PURE__*/React.createElement("div", {
    className: "fg-task__foot"
  }, assignee ? /*#__PURE__*/React.createElement("span", {
    className: "fg-assignee"
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-avatar"
  }, assignee.initials), assignee.name) : /*#__PURE__*/React.createElement("span", null), due && /*#__PURE__*/React.createElement("span", {
    className: cx("fg-due", overdue && "fg-due--overdue")
  }, due)));
}

/** .fg-kanban__col — one board column; count defaults to the number of children. */
function KanbanColumn({
  title,
  count,
  className,
  children
}) {
  const n = count ?? React.Children.count(children);
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-kanban__col", className)
  }, /*#__PURE__*/React.createElement("div", {
    className: "fg-kanban__head"
  }, /*#__PURE__*/React.createElement("span", {
    className: "fg-kanban__title"
  }, title), /*#__PURE__*/React.createElement("span", {
    className: "fg-kanban__count"
  }, n)), children);
}

/** .fg-kanban — horizontally scrolling board of KanbanColumns. */
function Kanban({
  className,
  children
}) {
  return /*#__PURE__*/React.createElement("div", {
    className: cx("fg-kanban", className)
  }, children);
}
Object.assign(__ds_scope, { TaskCard, KanbanColumn, Kanban });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/task/TaskCard.jsx", error: String((e && e.message) || e) }); }

// contract/elements.js
try { (() => {
/* ============================================================
   CANONICAL DESIGN CONTRACT — elements.js  (Tier-4 companion)

   OPTIONAL. Additive only: locks the DOM structure of the
   highest-reuse / highest-drift-risk roles into real custom
   elements, so pages compose them by TAG instead of hand-typing
   markup each time. Every element renders the SAME `.fg-*`
   classes/parts documented in SPEC.md and used by cards/demo —
   this is not a second implementation, it is the contract's own
   markup, emitted by code instead of by hand.

   - Renders into LIGHT DOM (no shadow root) — the page's linked
     styles.css (contract → components → patterns → theme) applies
     normally, so theme-switching still needs zero markup change.
   - Raw `.fg-*` markup keeps working untouched. Nothing here is
     required — delete this file and every page still renders,
     just back to hand-authored markup.
   - Load AFTER styles.css, once per page:
       <script src="elements.js"></script>

   Elements in this file: fg-button · fg-chip · fg-badge ·
   fg-status · fg-switch · fg-kpi · fg-card · fg-banner ·
   fg-caveat · fg-field · fg-avatar · fg-avatar-cluster ·
   fg-tabs / fg-tab.

   Not covered (compose from core roles + raw markup instead —
   content varies too much per page to lock into one shape):
   table, modal, cmdk, kanban, task-table, and all domain patterns.
   ============================================================ */
(function () {
  'use strict';

  // Detach and return this element's children matching `slotName`
  // ('' / undefined = the default slot: everything WITHOUT a
  // slot="" attribute, including bare text). Call the named-slot
  // extraction BEFORE the default one so it isn't swept up by it.
  function take(host, slotName) {
    const out = [];
    [...host.childNodes].forEach(node => {
      const named = node.nodeType === 1 && node.hasAttribute && node.hasAttribute('slot') ? node.getAttribute('slot') : null;
      if (slotName) {
        if (named === slotName) out.push(node);
      } else if (!named) {
        out.push(node);
      }
    });
    out.forEach(n => n.remove());
    return out;
  }
  function makeEl(tag, className) {
    const e = document.createElement(tag);
    if (className) e.className = className;
    return e;
  }
  function once(proto) {
    const orig = proto.connectedCallback;
    proto.connectedCallback = function () {
      if (this._fgBuilt) return;
      this._fgBuilt = true;
      // Parser-created instances (the primary use case: hand-authored static
      // HTML) fire connectedCallback as part of the parser's per-element
      // custom-element-reactions processing \u2014 BEFORE this element's later
      // children (e.g. an <input> appearing after other content) have been
      // parsed and appended. A microtask is NOT enough: microtask checkpoints
      // run repeatedly *during* synchronous parsing, not only once at the end,
      // so `queueMicrotask` can still fire mid-parse. A macrotask (setTimeout)
      // is scheduled after the parser's current task \u2014 in practice after the
      // whole synchronous parse for a static page \u2014 so children are present
      // by the time we read them. JS-created instances (already fully built
      // before insertion) just pay a harmless extra tick.
      setTimeout(() => orig.call(this), 0);
    };
  }

  // This platform's editor instrumentation tracks the ORIGINAL authored
  // text of an element and expects it to stay a direct child of whatever
  // it stamped — re-parenting that text into a newly created nested
  // element (e.g. wrapping a label in a fresh <button>) races with it and
  // leaves a duplicate floating copy behind. So for button/chip-toggle/
  // switch/tab below, the HOST element itself becomes the styled,
  // interactive control (classes + role + keyboard handling added
  // directly onto it) instead of nesting a second real <button> and
  // moving the label into it. Authored text never moves.
  function injectOnce(id, css) {
    if (document.getElementById(id)) return;
    const style = document.createElement('style');
    style.id = id;
    style.textContent = css;
    document.head.appendChild(style);
  }
  injectOnce('fg-elements-style', ['fg-switch { font-size: var(--type-body-sm-size); color: var(--color-text); }', 'fg-switch:focus-visible, fg-chip[toggle]:focus-visible { outline: var(--focus-width) solid var(--focus-color); outline-offset: var(--focus-offset); }'].join('\n'));

  // ---------------------------------------------------------- button
  class FgButton extends HTMLElement {
    connectedCallback() {
      const variant = this.getAttribute('variant') || 'primary';
      this.classList.add('fg-btn', 'fg-btn--' + variant);
      if (!this.hasAttribute('role')) this.setAttribute('role', 'button');
      this.addEventListener('keydown', e => {
        if (this.hasAttribute('disabled')) return;
        if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
          e.preventDefault();
          this.click();
        }
      });
      this.addEventListener('click', e => {
        if (this.hasAttribute('disabled')) {
          e.stopImmediatePropagation();
          e.preventDefault();
        }
      }, true);
      this._syncDisabled();
      this._ready = true;
    }
    static get observedAttributes() {
      return ['disabled', 'variant'];
    }
    attributeChangedCallback(name, _ov, nv) {
      if (!this._ready) return;
      if (name === 'variant') {
        [...this.classList].filter(c => c.indexOf('fg-btn--') === 0).forEach(c => this.classList.remove(c));
        this.classList.add('fg-btn--' + (nv || 'primary'));
      }
      if (name === 'disabled') this._syncDisabled();
    }
    _syncDisabled() {
      const dis = this.hasAttribute('disabled');
      this.setAttribute('aria-disabled', dis ? 'true' : 'false');
      this.tabIndex = dis ? -1 : 0;
      this.style.opacity = dis ? '.5' : '';
      this.style.cursor = dis ? 'not-allowed' : '';
      this.style.pointerEvents = dis ? 'none' : '';
    }
  }
  once(FgButton.prototype);
  customElements.define('fg-button', FgButton);

  // ---------------------------------------------------------- chip
  class FgChip extends HTMLElement {
    connectedCallback() {
      const tone = this.getAttribute('tone') || 'neutral';
      const toggle = this.hasAttribute('toggle');
      if (toggle) {
        this.classList.add('fg-chip', 'fg-chip--toggle');
        if (this.hasAttribute('on')) this.classList.add('fg-chip--on');
        this.setAttribute('role', 'button');
        this.tabIndex = 0;
        const fire = () => {
          const on = this.classList.toggle('fg-chip--on');
          if (on) this.setAttribute('on', '');else this.removeAttribute('on');
          this.dispatchEvent(new CustomEvent('fg-change', {
            detail: {
              on
            },
            bubbles: true
          }));
        };
        this.addEventListener('click', fire);
        this.addEventListener('keydown', e => {
          if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
            e.preventDefault();
            fire();
          }
        });
      } else {
        this.classList.add('fg-chip', 'fg-chip--' + tone);
      }
    }
  }
  once(FgChip.prototype);
  customElements.define('fg-chip', FgChip);

  // ---------------------------------------------------------- badge
  class FgBadge extends HTMLElement {
    connectedCallback() {
      const accent = this.getAttribute('tone') === 'accent';
      const content = take(this, '');
      this.classList.add('fg-badge');
      if (accent) this.classList.add('fg-badge--accent');
      content.forEach(n => this.append(n));
    }
  }
  once(FgBadge.prototype);
  customElements.define('fg-badge', FgBadge);

  // ---------------------------------------------------------- status
  class FgStatus extends HTMLElement {
    connectedCallback() {
      const state = this.getAttribute('state') || 'ready'; // ready | warn | blocked
      const label = take(this, '');
      this.classList.add('fg-status', 'fg-status--' + state);
      this.append(makeEl('span', 'fg-status__dot'));
      label.forEach(n => this.append(n));
    }
  }
  once(FgStatus.prototype);
  customElements.define('fg-status', FgStatus);

  // ---------------------------------------------------------- switch
  class FgSwitch extends HTMLElement {
    connectedCallback() {
      const labelAttr = this.getAttribute('label');
      this.classList.add('fg-switch');
      if (this.hasAttribute('on')) this.classList.add('fg-switch--on');
      this.setAttribute('role', 'switch');
      this.setAttribute('aria-checked', this.hasAttribute('on') ? 'true' : 'false');
      this.tabIndex = 0;
      const track = makeEl('span', 'fg-switch__track');
      track.append(makeEl('span', 'fg-switch__dot'));
      this.prepend(track); // authored label text (if any) stays put; track just moves to the front
      if (labelAttr != null) {
        const labelSpan = makeEl('span', 'fg-switch__label');
        labelSpan.textContent = labelAttr;
        this.append(labelSpan);
      }
      const fire = () => {
        const on = this.classList.toggle('fg-switch--on');
        this.setAttribute('aria-checked', on ? 'true' : 'false');
        if (on) this.setAttribute('on', '');else this.removeAttribute('on');
        this.dispatchEvent(new CustomEvent('fg-change', {
          detail: {
            on
          },
          bubbles: true
        }));
      };
      this.addEventListener('click', fire);
      this.addEventListener('keydown', e => {
        if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
          e.preventDefault();
          fire();
        }
      });
    }
  }
  once(FgSwitch.prototype);
  customElements.define('fg-switch', FgSwitch);

  // ---------------------------------------------------------- kpi
  class FgKpi extends HTMLElement {
    connectedCallback() {
      const label = this.getAttribute('label') || '';
      const value = this.getAttribute('value') || '';
      const deltaText = this.getAttribute('delta');
      const deltaDir = this.getAttribute('delta-dir') || 'flat'; // up | down | flat
      this.classList.add('fg-kpi');
      const l = makeEl('span', 'fg-kpi__label t-label');
      l.textContent = label;
      const v = makeEl('span', 'fg-kpi__value');
      v.textContent = value;
      this.append(l, v);
      if (deltaText) {
        const foot = makeEl('span', 'fg-kpi__foot');
        const d = makeEl('span', 'fg-delta fg-delta--' + deltaDir);
        d.textContent = deltaText;
        foot.append(d);
        this.append(foot);
      }
    }
  }
  once(FgKpi.prototype);
  customElements.define('fg-kpi', FgKpi);

  // ---------------------------------------------------------- card
  class FgCard extends HTMLElement {
    connectedCallback() {
      const title = this.getAttribute('title');
      const sub = this.getAttribute('sub');
      const rule = this.hasAttribute('rule');
      const sunken = this.hasAttribute('sunken');
      const foot = take(this, 'foot');
      const body = take(this, '');
      this.classList.add('fg-card');
      if (rule) this.classList.add('fg-card--rule');
      if (sunken) this.classList.add('fg-card--sunken');
      if (title) {
        const head = makeEl('div', 'fg-card__head');
        const t = makeEl('span', 'fg-card__title');
        t.textContent = title;
        head.append(t);
        this.append(head);
      }
      if (sub) {
        const s = makeEl('p', 'fg-card__sub t-body-sm');
        s.textContent = sub;
        this.append(s);
      }
      // Author-provided body/foot content must stay a DIRECT child of the
      // host (this platform's editor instrumentation re-homes it there) —
      // append directly instead of wrapping in fresh .fg-card__body/__foot
      // divs. Trade-off: body/foot content follows the card's own
      // --space-4 rhythm rather than the wrapper's tighter/row layout.
      body.forEach(n => this.append(n));
      foot.forEach(n => this.append(n));
    }
  }
  once(FgCard.prototype);
  customElements.define('fg-card', FgCard);

  // ---------------------------------------------------------- banner
  class FgBanner extends HTMLElement {
    connectedCallback() {
      const tone = this.getAttribute('tone') || 'info'; // info | warning | danger | success
      const actionLabel = this.getAttribute('action-label');
      const actionHref = this.getAttribute('action-href') || '#';
      const body = take(this, '');
      this.classList.add('fg-banner', 'fg-banner--' + tone);
      const dot = makeEl('span', 'fg-banner__dot');
      dot.style.order = '-1'; // pin first: immune to the platform re-homing tracked siblings
      this.append(dot);
      // Author-provided message must stay a direct child (see fg-card note
      // above) — append directly instead of wrapping in a fresh
      // .fg-banner__body span. It keeps the flex default `order: 0`.
      body.forEach(n => this.append(n));
      if (actionLabel) {
        const a = makeEl('a', 'fg-banner__act');
        a.href = actionHref;
        a.textContent = actionLabel;
        a.style.order = '1'; // pin last — DOM append order alone isn't honored (see note above)
        this.append(a);
      }
    }
  }
  once(FgBanner.prototype);
  customElements.define('fg-banner', FgBanner);

  // ---------------------------------------------------------- caveat
  class FgCaveat extends HTMLElement {
    connectedCallback() {
      const tone = this.getAttribute('tone') || 'info'; // info | warn | rule
      const body = take(this, '');
      this.classList.add('fg-caveat', 'fg-caveat--' + tone);
      body.forEach(n => this.append(n));
    }
  }
  once(FgCaveat.prototype);
  customElements.define('fg-caveat', FgCaveat);

  // ---------------------------------------------------------- field
  class FgField extends HTMLElement {
    connectedCallback() {
      const labelText = this.getAttribute('label');
      const required = this.hasAttribute('required');
      const hint = this.getAttribute('hint');
      const error = this.getAttribute('error');
      const control = take(this, ''); // author-provided <input>/<select>/<textarea>
      this.classList.add('fg-field');
      if (labelText) {
        const l = makeEl('label', 'fg-field__label t-label');
        l.style.order = '-1'; // pin first — see fg-banner note on `order` vs DOM position
        l.append(document.createTextNode(labelText + (required ? ' ' : '')));
        if (required) {
          const r = makeEl('span', 'fg-field__req');
          r.textContent = '*';
          l.append(r);
        }
        this.append(l);
      }
      control.forEach(c => {
        if (c.nodeType !== 1) {
          this.append(c);
          return;
        }
        if (c.tagName === 'SELECT') {
          // Skip the .fg-select wrapper + custom chevron (would wrap
          // author-provided content in a fresh element — same bug as
          // fg-card/fg-banner above). Falls back to the select's native
          // appearance; still fully functional.
          this.append(c);
        } else {
          c.classList.add('fg-input');
          if (c.tagName === 'TEXTAREA') c.classList.add('fg-input--area');
          this.append(c);
        }
      });
      if (hint && !error) {
        const h = makeEl('span', 'fg-field__hint');
        h.style.order = '1'; // pin last — the tracked <input>/<select> ends up appended before this
        h.textContent = hint;
        this.append(h);
      }
      if (error) {
        const e = makeEl('span', 'fg-field__error');
        e.style.order = '1';
        e.textContent = error;
        this.append(e);
      }
    }
  }
  once(FgField.prototype);
  customElements.define('fg-field', FgField);

  // ---------------------------------------------------------- avatar
  class FgAvatar extends HTMLElement {
    connectedCallback() {
      const more = this.hasAttribute('more');
      const content = take(this, '');
      this.classList.add('fg-avatar');
      if (more) this.classList.add('fg-avatar--more');
      content.forEach(n => this.append(n));
    }
  }
  once(FgAvatar.prototype);
  customElements.define('fg-avatar', FgAvatar);
  class FgAvatarCluster extends HTMLElement {
    connectedCallback() {
      this.classList.add('fg-avatar-cluster');
    }
  }
  once(FgAvatarCluster.prototype);
  customElements.define('fg-avatar-cluster', FgAvatarCluster);

  // ---------------------------------------------------------- tabs / tab
  class FgTab extends HTMLElement {
    connectedCallback() {
      this.classList.add('fg-tab');
      if (this.hasAttribute('active')) this.classList.add('fg-tab--on');
      this.setAttribute('role', 'tab');
      this.tabIndex = 0;
      this.addEventListener('click', () => this._activate());
      this.addEventListener('keydown', e => {
        if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
          e.preventDefault();
          this._activate();
        }
      });
    }
    _activate() {
      const tabs = this.closest('fg-tabs');
      if (tabs) {
        [...tabs.children].forEach(t => {
          if (t.tagName === 'FG-TAB' && t !== this) {
            t.classList.remove('fg-tab--on');
            t.removeAttribute('active');
          }
        });
      }
      this.classList.add('fg-tab--on');
      this.setAttribute('active', '');
      if (tabs) {
        tabs.dispatchEvent(new CustomEvent('fg-tab-change', {
          detail: {
            label: this.textContent.trim()
          },
          bubbles: true
        }));
      }
    }
  }
  once(FgTab.prototype);
  customElements.define('fg-tab', FgTab);
  class FgTabs extends HTMLElement {
    connectedCallback() {
      this.classList.add('fg-tabs');
    }
  }
  once(FgTabs.prototype);
  customElements.define('fg-tabs', FgTabs);
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "contract/elements.js", error: String((e && e.message) || e) }); }

// demo/theme-switcher.js
try { (() => {
/* ============================================================
   UNIFIED THEME SWITCHER — shared by every demo page.
   - Applies theme/accent/density/num-font from localStorage
     immediately (before paint) to avoid a flash.
   - Builds an identical control bar + page nav into #fg-bar on
     DOMContentLoaded.
   - All four axes persist and stay in sync across pages.
   ============================================================ */
(function () {
  // Only act when loaded as demo/theme-switcher.js — never when swept into a compiled bundle.
  var cs = document.currentScript;
  if (!cs || !/theme-switcher\.js(\?|$)/.test(cs.src)) return;
  var html = document.documentElement;
  var PAGES = [{
    href: "core.html",
    label: "Core"
  }, {
    href: "editorial.html",
    label: "Editorial"
  }, {
    href: "task.html",
    label: "Task"
  }, {
    href: "crm.html",
    label: "CRM"
  }, {
    href: "financial.html",
    label: "Financial"
  }, {
    href: "media.html",
    label: "Media"
  }, {
    href: "elements.html",
    label: "Elements"
  }, {
    href: "contract-tests.html",
    label: "Tests"
  }];
  var ACCENTS = {
    clickup: [["", "Ink"], ["violet", "Violet"], ["blue", "Blue"]],
    precision: [["", "Amber"], ["moss", "Moss"], ["honey", "Honey"]],
    berich: [["", "Jade"], ["gold", "Gold"], ["sapphire", "Sapphire"], ["topaz", "Topaz"], ["amethyst", "Amethyst"], ["garnet", "Garnet"]],
    terminal: [["", "Green"], ["amber", "Amber"]],
    atelier: [["", "Ember"], ["clay", "Clay"], ["honey", "Honey"]],
    moday: [["", "Blue"], ["purple", "Purple"], ["green", "Green"]]
  };

  // Per-theme typeface-sets. "" = the theme's own default families;
  // "system" is the universal zero-download set defined in contract.css.
  var TYPEFACES = {
    clickup: [["", "Geometric"], ["grotesk-tight", "Grotesk"], ["editorial-pop", "Space"], ["system", "System"]],
    precision: [["", "Editorial"], ["grotesk", "Grotesk"], ["mono-lab", "Mono Lab"], ["system", "System"]],
    berich: [["", "Signature"], ["humanist", "Humanist"], ["grotesk-warm", "Grotesk"], ["system", "System"]],
    terminal: [["", "JetBrains"], ["plex", "IBM Plex"], ["sometype", "Sometype"], ["system", "System"]],
    atelier: [["", "Manrope"], ["grotesk", "Grotesk"], ["system", "System"]],
    moday: [["", "Figtree + Poppins"], ["figtree", "Figtree"], ["system", "System"]]
  };
  var get = function (k, d) {
    var v = localStorage.getItem("fg-" + k);
    if (k === "theme" && v === "vibe") v = "moday";
    return v || d;
  };
  var set = function (k, v) {
    if (v) localStorage.setItem("fg-" + k, v);else localStorage.removeItem("fg-" + k);
  };

  // Each theme's NATIVE color scheme. Switching theme resets scheme to this
  // (the theme's intended look); the Scheme segment then forces light/dark.
  var NATIVE = {
    clickup: "light",
    precision: "dark",
    berich: "light",
    terminal: "dark",
    atelier: "light",
    moday: "light"
  };
  var nativeScheme = function () {
    return NATIVE[get("theme", "clickup")] || "light";
  };
  function apply() {
    html.setAttribute("data-theme", get("theme", "clickup"));
    html.setAttribute("data-scheme", get("scheme", nativeScheme()));
    var a = get("accent", "");
    a ? html.setAttribute("data-accent", a) : html.removeAttribute("data-accent");
    var d = get("density", "");
    d ? html.setAttribute("data-density", d) : html.removeAttribute("data-density");
    var n = get("num", "");
    n ? html.setAttribute("data-num-font", n) : html.removeAttribute("data-num-font");
    var t = get("typeface", "");
    t ? html.setAttribute("data-typeface-set", t) : html.removeAttribute("data-typeface-set");
  }
  apply(); // run ASAP (in <head>) to prevent theme flash

  function segment(label, options, current, onPick) {
    var grp = document.createElement("div");
    grp.className = "demo-grp";
    var lbl = document.createElement("span");
    lbl.className = "demo-grp__lbl";
    lbl.textContent = label;
    grp.appendChild(lbl);
    var seg = document.createElement("div");
    seg.className = "seg";
    options.forEach(function (opt) {
      var b = document.createElement("button");
      b.dataset.v = opt[0];
      b.textContent = opt[1];
      b.setAttribute("aria-pressed", opt[0] === current);
      b.onclick = function () {
        [].forEach.call(seg.children, function (c) {
          c.setAttribute("aria-pressed", c.dataset.v === opt[0]);
        });
        onPick(opt[0]);
      };
      seg.appendChild(b);
    });
    grp.appendChild(seg);
    return grp;
  }
  var PALETTE_ICON = '<svg width="17" height="17" viewBox="0 0 17 17" fill="none" xmlns="http://www.w3.org/2000/svg">' + '<path d="M8.5 1.5c-3.87 0-7 3.13-7 7s3.13 7 7 7c.83 0 1.5-.67 1.5-1.5 0-.4-.16-.76-.41-1.03-.25-.27-.41-.63-.41-1.03 0-.83.67-1.5 1.5-1.5h1.77c1.72 0 3.11-1.4 3.11-3.11C15.5 4.03 12.42 1.5 8.5 1.5z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>' + '<circle cx="5" cy="7.2" r="1" fill="currentColor"/>' + '<circle cx="8.3" cy="4.8" r="1" fill="currentColor"/>' + '<circle cx="11.8" cy="6.6" r="1" fill="currentColor"/>' + '<circle cx="5.3" cy="10.8" r="1" fill="currentColor"/>' + '</svg>';
  function build() {
    var host = document.getElementById("fg-bar");
    if (!host) return;
    host.className = "demo-bar";
    host.innerHTML = "";

    // page nav — centered
    var here = location.pathname.split("/").pop() || "core.html";
    var nav = document.createElement("nav");
    nav.className = "demo-nav";
    PAGES.forEach(function (p) {
      var a = document.createElement("a");
      a.href = p.href;
      a.textContent = p.label;
      if (p.href === here) a.setAttribute("aria-current", "page");
      nav.appendChild(a);
    });

    // left spacer (keeps nav visually centered against the palette trigger)
    var spacer = document.createElement("div");
    spacer.className = "demo-bar__spacer";

    // palette trigger + popover with all customizer controls
    var wrap = document.createElement("div");
    wrap.className = "demo-palette";
    var trigger = document.createElement("button");
    trigger.className = "demo-palette__btn";
    trigger.type = "button";
    trigger.title = "Customize theme";
    trigger.setAttribute("aria-label", "Customize theme");
    trigger.innerHTML = PALETTE_ICON;
    var pop = document.createElement("div");
    pop.className = "demo-palette__pop";
    pop.hidden = true;
    var ctrls = document.createElement("div");
    ctrls.className = "demo-ctrls";

    // theme
    ctrls.appendChild(segment("Theme", [["clickup", "ClickUp"], ["precision", "Precision"], ["berich", "beRich"], ["terminal", "Terminal*"], ["atelier", "Atelier"], ["moday", "Moday"]], get("theme", "clickup"), function (v) {
      set("theme", v);
      set("accent", "");
      set("typeface", "");
      set("scheme", NATIVE[v] || "light");
      apply();
      rebuildScheme();
      rebuildAccent();
      rebuildTypeface();
    }));

    // scheme (light/dark) — right after theme; resets to the theme's native
    // scheme on theme change, so this slot is rebuildable
    var schemeSlot = document.createElement("div");
    schemeSlot.id = "fg-scheme-slot";
    schemeSlot.style.display = "contents";
    ctrls.appendChild(schemeSlot);

    // accent (depends on theme) — rebuildable slot
    var accentSlot = document.createElement("div");
    accentSlot.id = "fg-accent-slot";
    accentSlot.style.display = "contents";
    ctrls.appendChild(accentSlot);

    // typeface-set (depends on theme) — rebuildable slot
    var typeSlot = document.createElement("div");
    typeSlot.id = "fg-type-slot";
    typeSlot.style.display = "contents";
    ctrls.appendChild(typeSlot);

    // density
    ctrls.appendChild(segment("Density", [["", "Default"], ["compact", "Compact"]], get("density", ""), function (v) {
      set("density", v);
      apply();
    }));

    // numbers
    ctrls.appendChild(segment("Numbers", [["mono", "Mono"], ["sans", "Sans"]], get("num", "mono"), function (v) {
      set("num", v === "mono" ? "" : v);
      apply();
    }));
    pop.appendChild(ctrls);
    wrap.appendChild(trigger);
    wrap.appendChild(pop);
    trigger.onclick = function (e) {
      e.stopPropagation();
      pop.hidden = !pop.hidden;
      trigger.setAttribute("aria-pressed", !pop.hidden);
    };
    document.addEventListener("click", function (e) {
      if (!pop.hidden && !wrap.contains(e.target)) {
        pop.hidden = true;
        trigger.setAttribute("aria-pressed", "false");
      }
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && !pop.hidden) {
        pop.hidden = true;
        trigger.setAttribute("aria-pressed", "false");
        trigger.focus();
      }
    });
    host.appendChild(spacer);
    host.appendChild(nav);
    host.appendChild(wrap);
    rebuildScheme();
    rebuildAccent();
    rebuildTypeface();
    function rebuildScheme() {
      var slot = document.getElementById("fg-scheme-slot");
      slot.innerHTML = "";
      slot.appendChild(segment("Scheme", [["light", "Light"], ["dark", "Dark"]], get("scheme", nativeScheme()), function (v) {
        set("scheme", v);
        apply();
      }));
    }
    function rebuildAccent() {
      var slot = document.getElementById("fg-accent-slot");
      slot.innerHTML = "";
      var theme = get("theme", "clickup");
      slot.appendChild(segment("Accent", ACCENTS[theme], get("accent", ""), function (v) {
        set("accent", v);
        apply();
      }));
    }
    function rebuildTypeface() {
      var slot = document.getElementById("fg-type-slot");
      slot.innerHTML = "";
      var theme = get("theme", "clickup");
      slot.appendChild(segment("Typeface", TYPEFACES[theme], get("typeface", ""), function (v) {
        set("typeface", v);
        apply();
      }));
    }
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", build);else build();
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "demo/theme-switcher.js", error: String((e && e.message) || e) }); }

__ds_ns.Button = __ds_scope.Button;

__ds_ns.Profile = __ds_scope.Profile;

__ds_ns.Facts = __ds_scope.Facts;

__ds_ns.Pipeline = __ds_scope.Pipeline;

__ds_ns.Table = __ds_scope.Table;

__ds_ns.Avatar = __ds_scope.Avatar;

__ds_ns.AvatarCluster = __ds_scope.AvatarCluster;

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Card = __ds_scope.Card;

__ds_ns.Chip = __ds_scope.Chip;

__ds_ns.Kpi = __ds_scope.Kpi;

__ds_ns.Status = __ds_scope.Status;

__ds_ns.Banner = __ds_scope.Banner;

__ds_ns.Caveat = __ds_scope.Caveat;

__ds_ns.Ledger = __ds_scope.Ledger;

__ds_ns.Field = __ds_scope.Field;

__ds_ns.Input = __ds_scope.Input;

__ds_ns.Select = __ds_scope.Select;

__ds_ns.Switch = __ds_scope.Switch;

__ds_ns.NavItem = __ds_scope.NavItem;

__ds_ns.NavRail = __ds_scope.NavRail;

__ds_ns.Tabs = __ds_scope.Tabs;

__ds_ns.Tab = __ds_scope.Tab;

__ds_ns.TaskCard = __ds_scope.TaskCard;

__ds_ns.KanbanColumn = __ds_scope.KanbanColumn;

__ds_ns.Kanban = __ds_scope.Kanban;

})();
