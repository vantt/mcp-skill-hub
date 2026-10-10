export const en = {
  'app.title': 'Skill Hub',
  // Navigation
  'nav.home': 'Home',
  'nav.skills': 'Skills',
  'nav.sources': 'Sources',
  'nav.skip_to_main': 'Skip to main content',

  // Appearance
  'appearance.menu_title': 'Appearance',
  'appearance.scheme': 'Color scheme',
  'appearance.scheme_light': 'Light',
  'appearance.scheme_dark': 'Dark',
  'appearance.scheme_system': 'System',
  'appearance.theme': 'Theme',
  'appearance.accent': 'Accent',

  // Workspace
  'workspace.title': 'Workspace',
  'workspace.row_health': 'Health',
  'workspace.row_index': 'Index',
  'workspace.row_git': 'Git',
  'workspace.status_valid': 'Healthy',
  'workspace.status_invalid': 'Invalid',
  'workspace.status_recovering': 'Recovery required',
  'workspace.index_current': 'Index current',
  'workspace.index_stale': 'Index stale',
  'workspace.git_clean': 'Git clean',
  'workspace.git_dirty': 'Git uncommitted changes',
  'workspace.unavailable': 'Unavailable',

  // Lifecycle
  'lifecycle.all': 'All',
  'lifecycle.draft': 'Draft — Not yet usable by agents',
  'lifecycle.active': 'Active — Routable',
  'lifecycle.deprecated': 'Deprecated — Not preferred for new work',
  'lifecycle.archived': 'Archived — Removed from routing',

  // Lifecycle short
  'lifecycle_short.draft': 'Draft',
  'lifecycle_short.active': 'Active',
  'lifecycle_short.deprecated': 'Deprecated',
  'lifecycle_short.archived': 'Archived',

  // Routing
  'routing.routable': 'Routable',
  'routing.not_routed': 'Not routed',

  // Category availability
  'availability.available': 'Available',
  'availability.not_configured': 'Not configured in v1',
  'availability.unavailable': 'Unavailable until the workspace and index are ready',

  // Source status
  'source_status.unreachable': 'Unreachable (last check)',
  'source_status.changed': 'Changed',
  'source_status.distill_pending': 'Distill pending',
  'source_status.never_distilled': 'Never distilled',
  'source_status.up_to_date': 'Up to date',

  // Actions / CTAs
  'action.copy_command': 'Copy command',
  'action.copied': 'Copied',
  'action.open_sources': 'Open sources',
  'action.check_all_sources': 'Check all sources',
  'action.check_due_sources': 'Check due sources',
  'action.add_from_github': 'Add from GitHub',
  'action.create_skill': 'Create skill',
  'action.review': 'Review',
  'action.deprecate': 'Deprecate',
  'action.archive': 'Archive',
  'action.clear_filters': 'Clear filters',
  'action.cancel': 'Cancel',
  'action.back': 'Back',
  'action.close': 'Close',
  'action.reload': 'Reload',
  'action.review_upstream_updates': 'Review updates →',
  'action.track_upstream_skills': 'Copy command',
  'action.link_orphan_sources': 'Open sources →',
  'action.distill_with_curator': 'Distill with Curator Agent',
  'action.copy_handoff': 'Copy handoff',
  'action.refresh_status': 'Refresh status',
  // Home Screen
  'home.title': 'Home',
  'home.subtitle': 'Curate and monitor your Skill Hub workspace.',
  'home.next_action_title': 'Next action',
  'home.nothing_needs_attention': 'Nothing needs attention.',
  'home.degraded_title': 'Search index needs rebuild.',
  'home.degraded_body': 'Catalog is stale. Run skillhub rebuild, then reload.',
  'home.summary_title': 'Overview',
  'home.not_configured': 'Not set up',
  'home.category.sources_ready_to_distill': 'Sources ready to distill',
  'home.category.candidate_lessons': 'Candidate lessons to review',
  'home.category.source_unavailable': 'Unreachable sources',
  'home.category.sources_due': 'Sources due for a check',
  'home.category.blocking_decisions': 'Blocking decisions',
  'home.category.routing_evaluations': 'Routing evaluations',
  'home.unreachable_sources.one': '{count} unreachable source',
  'home.unreachable_sources.other': '{count} unreachable sources',

  // Skills Screen
  'skills.title': 'Skills',
  'skills.search_placeholder': 'Search id or name…',
  'skills.filter_lifecycle': 'Lifecycle',
  'skills.filter_collection': 'Collection',
  'skills.filter_upstream': 'Upstream',
  'skills.search_label': 'Search',
  'skills.col_skill': 'Skill',
  'skills.col_collection': 'Collection',
  'skills.col_lifecycle': 'Lifecycle',
  'skills.col_routing': 'Routing',
  'skills.col_actions': 'Actions',
  'skills.empty_none': 'No skills yet.',
  'skills.empty_filtered': 'No skills match these filters.',
  'skills.fallback_banner_prefix': 'Using fallback index:',

  // Common Messages
  'common.loading': 'Loading…',
  'common.error': 'An error occurred',
  'common.later_phase': 'This screen arrives in a later delivery phase.',
  'common.not_found': 'Page not found.',
  'common.not_found_hint':
    'This address does not lead anywhere. Lessons to review are in the Distill tab of each skill; sources are on the Sources page.',
  'common.session_expired_title': 'Session expired',
  'common.session_expired_desc': 'Your session token is missing or has expired. Please reopen the web UI from the terminal command `skillhub serve web`.',
} as const;
