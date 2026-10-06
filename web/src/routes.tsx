import type { RouteObject } from 'react-router';
import { NotFoundPage } from './components/NotFoundPage';
import { HomeScreen } from './screens/home/HomeScreen';
import { SkillsScreen } from './screens/skills/SkillsScreen';
import { AddSkillScreen } from './screens/skill-add/AddSkillScreen';
import { CreateSkillScreen } from './screens/skill-create/CreateSkillScreen';
import { SkillDetailScreen } from './screens/skill-detail/SkillDetailScreen';
import { SourcesScreen } from './screens/sources/SourcesScreen';
import { DistillHandoffScreen } from './screens/distill/DistillHandoffScreen';
import { RunScreen } from './screens/run/RunScreen';
import { InboxScreen } from './screens/inbox/InboxScreen';
import { InsightDetailScreen } from './screens/insight/InsightDetailScreen';
import { PatchComposerScreen } from './screens/composer/PatchComposerScreen';
export const routes: RouteObject[] = [
  {
    path: '/',
    Component: HomeScreen,
  },
  {
    path: '/skills',
    Component: SkillsScreen,
  },
  {
    path: '/skills/add',
    Component: AddSkillScreen,
  },
  {
    path: '/skills/create',
    Component: CreateSkillScreen,
  },
  {
    path: '/skills/:id',
    Component: SkillDetailScreen,
  },
  {
    path: '/sources',
    Component: SourcesScreen,
  },
  {
    path: '/sources/distill',
    Component: DistillHandoffScreen,
  },
  {
    path: '/sources/runs/:id',
    Component: RunScreen,
  },
  {
    path: '/inbox',
    Component: InboxScreen,
  },
  {
    path: '/inbox/:id',
    Component: InsightDetailScreen,
  },
  {
    path: '/inbox/:id/apply',
    Component: PatchComposerScreen,
  },
  {
    path: '*',
    Component: NotFoundPage,
  },
];
