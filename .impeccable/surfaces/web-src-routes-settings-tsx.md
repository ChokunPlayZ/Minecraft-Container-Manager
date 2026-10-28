---
version: 1
slug: "web-src-routes-settings-tsx"
primary_target: "web/src/routes/settings.tsx"
related_targets: ["web/src/routes/user-settings.tsx", "web/src/components/settings-tabs.tsx", "web/src/components/app-shell.tsx"]
---

# App and user settings

Mode: Operate. Narrow extension of the existing settings interface. User request: "seperate app settings from user settings."

## Direction contract

Keep the existing mineral/graphite surfaces, forest-teal actions, system typography, shared cards, controls, and responsive shell. Separate settings by scope without changing their APIs or stored values.

The main navigation's App Settings entry opens `/settings`, defaulting to Updates. DNS Providers, Port Pool, and Notifications remain installation-wide tabs at their existing URLs. The profile link, mobile avatar, and expanded mobile menu open User Settings at `/user-settings?tab=account`, with Account Security and Appearance tabs. The profile entry is selected on the personal page and the workspace breadcrumb identifies User Settings.

The first viewport names the settings scope above the existing underlined tab strip and form panels. Desktop account cards retain their existing two-column arrangement; mobile stacks the cards and permits horizontal scrolling for the longer app tab strip. Tab selection updates the URL, and arrow keys, Home, and End move selection and keyboard focus.

Profile, password, two-factor authentication, and passkeys retain the signed-in user's existing endpoints. Appearance reuses the existing theme provider, persists only in this browser, and stays in sync with the header theme menu. `/settings?tab=account` redirects to the personal page, preserving other search parameters.

## Evidence

Desktop (1440px) and mobile (390px) dark-theme previews are saved in `.impeccable/review/settings-separation/`: `app-desktop.png`, `app-mobile.png`, `user-desktop.png`, `user-mobile.png`, and `appearance-mobile.png`. Each capture was opened and checked for the intended content. Mobile navigation and the legacy account URL were also inspected in the running preview. Demo authentication getters return illustrative disabled 2FA and an empty passkey list.

Nine route integration tests cover separation, preserved bookmarks, legacy redirects, profile saves, browser-local appearance, shared theme state, and keyboard navigation. Existing settings and theme tests cover the reused forms. Shared tokens, typography, and controls remain unchanged.

The implementing agent reports 47 settings/theme tests passed, followed by a passing typecheck, changed-file ESLint, production build, and `git diff --check`. The existing build chunk-size warning remains.

## Finish disposition

**Ship.** The independent finish review found no material regression or request gap and requested no implementation changes. It reviewed all five captures and the changed source; see `.impeccable/review/settings-separation/finish-review.md`. Durable navigation documentation and the matching sidecar demo label now reflect App Settings and the separate personal route. No visual tokens or unrelated design guidance changed.
