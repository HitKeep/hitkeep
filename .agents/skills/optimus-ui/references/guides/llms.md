# LLMs.txt

LLM-optimized documentation endpoints for Optimus UI components.

## Agentskill-

The markdown files above are also bundled as an Agent Skill , the SKILL.md format supported by coding agents such as Claude Code, Codex, Cursor and OpenCode. Once installed, the agent reads the component docs, API tables and guides from disk whenever it works on Optimus UI code, instead of fetching them from this site each time. Install it with the skills CLI , which asks which agents to set it up for. By default the skill is installed into the current project, so your team gets it too. Add -g to install it for your user instead. You can also download the zip and extract it into a folder named optimus-ui in your agent's skills directory.

```bash
npx skills add https://optimus.openng.org

# update to the latest docs
npx skills update optimus-ui
```

## Llmsfulltxt-

The llms-full.txt file is a complete list of all the pages in the Optimus UI documentation. It is used to help AI models understand the entire documentation set.

## Llmstxt-

The llms.txt file is an industry standard that helps AI models better understand and navigate the Optimus UI documentation. It lists key pages in a structured format, making it easier for LLMs to retrieve relevant information.

## Manifest-

/llms/manifest.json is the single source the files above, the Agent Skill and the MCP server are generated from. Use it for tools that need exact facts rather than prose, such as code generators, linters and design tool integrations. It joins two sources, each read from where it is defined: entryPoints : the API, read from the Angular metadata in the compiled library, so selectors and inputs match what the compiler accepts. Each entry point lists its import path, NgModule, services, exported types, and per component or directive its selector, inputs (type, allowed values, default, transform, two-way binding), outputs, templates, methods, forms support and pass-through keys. components and guides : the documentation pages with their sections and example code, linked to the entry points they document, and the design tokens of each component.

```typescript
{
  "schemaVersion": 1,
  "components": [{ "name": "button", "title": "Button", "entryPoints": ["button"], "sections": [...], "tokens": [{ "name": "button.primary.background", "variable": "--p-button-primary-background" }] }],
  "entryPoints": [
    {
      "name": "button",
      "import": "@openng/optimus-ui/button",
      "modules": ["ButtonModule"],
      "declarations": [
        {
          "kind": "component",
          "className": "Button",
          "selector": "p-button",
          "inputs": [
            { "name": "severity", "type": "ButtonSeverity", "values": ["success", "info", "warn", "danger", "secondary", "contrast", "help", "primary"] },
            { "name": "raised", "type": "boolean", "default": "false", "transform": "booleanAttribute" }
          ],
          "outputs": [{ "name": "onClick", "payload": "MouseEvent" }],
          "templates": [{ "name": "icon", "context": "ButtonIconTemplateContext" }]
        }
      ]
    }
  ]
}
```

## Markdownextension-

Add a .md to a page's URL to display a Markdown version of that page.

## Mcp-

&#64;openng/optimus-ui-mcp is a Model Context Protocol server that lets coding agents look up Optimus UI while they work: components by name or selector, their inputs, outputs and templates, the code of every docs example, types such as MenuItem and the guides. It runs locally over stdio and ships the manifest of its release, so it works offline and matches the version you install. Register it with your agent, for example Claude Code: Or add it to the MCP configuration of Cursor, VS Code, Windsurf or another client: The tools are search , list_components , get_component , get_example , get_type and get_guide . The Agent Skill above covers the same content as files; use either, or both.
