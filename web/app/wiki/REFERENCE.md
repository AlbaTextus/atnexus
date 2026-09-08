# Wiki layout reference

Reference project: https://github.com/chenyme/LINUX-DO-WIKI

The local reference checkout is kept under `temps/references/LINUX-DO-WIKI`.
This integration uses the same Nextra 4.2.17 and nextra-theme-docs 4.2.17
Layout, Navbar, Footer, Sidebar, TOC and document typography as that project.
Nextra's MIT-licensed packages supply those components. AlbaTextus content,
branding, live policy loading and search are implemented locally; unrelated
community articles, account integrations and image assets are not imported.

Policy Markdown is rendered without evaluating MDX/JavaScript from the database.
The existing management API remains the source of truth for policy content.
