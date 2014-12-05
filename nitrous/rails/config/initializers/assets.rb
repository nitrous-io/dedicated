Rails.application.config.assets.precompile += %w(jquery.js
                                                 terminal.js
                                                 terminal.css
                                                 homepage.css
                                                 pages.js
                                                 ide/app.js
                                                 glyphicons-halflings.png
                                                 glyphicons-halflings-white.png
                                                 ide.css)

# FIXME: Added to get IDE to work
# All Hamlbars templates must be ported to emblem.
Hamlbars::Template.render_templates_for :ember

# monkey patch hamlbars so that templates starting with underscore(_) are still treated as templates
module Hamlbars
  class Template
    def self.path_translator(path)
      path
    end

    def evaluate(scope, locals, &block)
      template = if @engine.respond_to?(:precompiled_method_return_value, true)
                   super(scope, locals, &block)
                 else
                   @engine.render(scope, locals, &block)
                 end

      if scope.respond_to? :logical_path
        path = scope.logical_path
      else
        path = basename
      end

      name = self.class.path_translator(path)
      "#{self.class.template_destination}[\"#{name}\"] = #{self.class.template_compiler}(\"#{template.strip.gsub(/(\r\n|[\n\r"'])/) { JS_ESCAPE_MAP[$1] }}\");\n"
    end
  end
end
