/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'intro',
    'getting-started',
    {
      type: 'category',
      label: 'How It Works',
      items: [
        'how-it-works/read-path',
        'how-it-works/write-path',
        'how-it-works/quality-loop',
        'how-it-works/hybrid-search',
      ],
    },
    {
      type: 'category',
      label: 'Agent Integration',
      items: [
        'agents/mcp-server',
        'agents/proxy-mode',
        'agents/read-only-mode',
      ],
    },
    'team-knowledge-hub',
    'configuration',
  ],
};

export default sidebars;
