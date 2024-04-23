# smolblog
The tiniest site generator I could think of.

## The Idea

A static site generator that is driven from a JSON file instead of a database.
The templates are rendered using Go's stdlib templates, which some people don't like,
but is entirely flexible and requires no external dependency.
The hiearchy of args passed to a template is rather flat, so nothing too obscure can be done:
arguments are a single level map, and pages are a single level array.

Most of the binary is a wrapper around existing Go functionality, but the idea
is that if you can produce the JSON, this can do the rest.

## Reuseable Templates

## Static Assets

# TODO's

- [x] MVP that renders my existing site (jamesholdren.com)
- [ ] Support for static assets (copy from one spot to another)
- [ ] Helper functions
- [ ] Post processors? (Support for CSS/JS minification)
- [ ] Status bar or something
