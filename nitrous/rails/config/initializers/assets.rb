Rails.application.config.assets.precompile += %w(
  jquery.js
  main.js
  terminal.js
  terminal.css
  homepage.css
  pages.js
  ide.css
  ide/main.js
)


Hamlbars::Template.render_templates_for :ember
