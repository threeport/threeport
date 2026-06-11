# Threeport User Documentation

User docs for Threeport.

## Live Updates

The live docs site is updated when either of the following things are done:

1. When a release is cut by pushing a tag with a semantic version, e.g. `v0.5.3`
2. When a tag is pushed that begins with `docs`.

Therefore, docs are updated with each release of Threeport.  Also, if you want
to make an update to the docs site without a Threeport release, push a `docs*`
tag.

## Local Development

Prerequisistes:

* [python 3](https://docs.python-guide.org/starting/installation/)
* [pip](https://pypi.org/project/pip/)

Run all the following commands from this `docs/` directory.

Create virtual environment and activate:

```bash
python3 -m venv .venv
source .venv/bin/activate
```

Install python requirements:

```bash
pip install -r requirements.txt
```

Run the server locally:

```bash
mkdocs serve
```

View the site at [http://127.0.0.1:8000](http://127.0.0.1:8000/)

## Diagrams

For consistency, use draw.io for all diagrams in user documentation. Add each diagram as an svg file with the extension `.drawio.svg`. This allows diagrams to be updated using the same draw.io tool.

> Note: if using the `hediet.vscode-drawio` vscode extension, create and modify the diagrams in light mode. If you use darkmode, the diagrams will lose all color when viewed on the docs site in light mode.

