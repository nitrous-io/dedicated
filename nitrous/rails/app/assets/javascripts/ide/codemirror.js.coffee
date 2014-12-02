#= require codemirror/codemirror
#= require codemirror/mode/clike/clike
#= require codemirror/mode/clojure/clojure
#= require codemirror/mode/coffeescript/coffeescript
#= require codemirror/mode/css/css
#= require codemirror/mode/diff/diff
#= require codemirror/mode/erlang/erlang
#= require codemirror/mode/groovy/groovy
#= require codemirror/mode/go/go
#= require codemirror/mode/htmlembedded/htmlembedded
#= require codemirror/mode/htmlmixed/htmlmixed
#= require codemirror/mode/javascript/javascript
#= require codemirror/mode/jinja2/jinja2
#= require codemirror/mode/lua/lua
#= require codemirror/mode/markdown/markdown
#= require codemirror/mode/perl/perl
#= require codemirror/mode/php/php
#= require codemirror/mode/python/python
#= require codemirror/mode/ruby/ruby
#= require codemirror/mode/sass/sass
#= require codemirror/mode/scheme/scheme
#= require codemirror/mode/shell/shell
#= require codemirror/mode/sql/sql
#= require codemirror/mode/xml/xml
#= require codemirror/mode/yaml/yaml
#= require codemirror/mode/haml/haml

#= require codemirror/keymap/vim
#= require codemirror/keymap/sublime

#= require codemirror/addon/comment/comment
#= require codemirror/addon/display/rulers
#= require codemirror/addon/edit/matchbrackets
#= require codemirror/addon/edit/closebrackets
#= require codemirror/addon/edit/trailingspace
#= require codemirror/addon/selection/active-line
#= require codemirror/addon/selection/mark-selection
#= require codemirror/addon/search/search
#= require codemirror/addon/search/searchcursor
#= require codemirror/addon/search/match-highlighter
#= require codemirror/addon/dialog/dialog.js

#= require codemirror/addon/hint/show-hint
#= require codemirror/addon/hint/anyword-hint
#= require codemirror/addon/hint/css-hint
#= require codemirror/addon/hint/xml-hint
#= require codemirror/addon/hint/html-hint
#= require codemirror/addon/hint/javascript-hint

#= require codemirror/addon/fold/foldcode
#= require codemirror/addon/fold/foldgutter
#= require codemirror/addon/fold/brace-fold
#= require codemirror/addon/fold/comment-fold
#= require codemirror/addon/fold/indent-fold
#= require codemirror/addon/fold/xml-fold

#= require codemirror/addon/lint/lint
#= require jshint
#= require codemirror/addon/lint/javascript-lint
#= require jsonlint
#= require codemirror/addon/lint/json-lint

CodeMirror.defineMIME 'text/x-jinja2', name: 'jinja2'

CodeMirror.MODES =
  'text/plain'              : 'Plain Text'
  'text/x-csrc'             : 'C'
  'text/x-c++src'           : 'C++'
  #'text/x-csharp'
  'text/x-clojure'          : 'Clojure'
  'text/x-coffeescript'     : 'CoffeeScript'
  'text/css'                : 'CSS'
  'text/x-diff'             : 'Diff'
  'text/x-erlang'           : 'Erlang'
  #'application/x-aspx'
  'text/x-go'               : 'Go'
  'text/x-groovy'           : 'Groovy'
  'text/x-haml'             : 'Haml'
  'text/html'               : 'HTML'
  'application/x-ejs'       : 'HTML (ejs)'
  'application/x-erb'       : 'HTML (erb)'
  'text/x-java'             : 'Java'
  'text/javascript'         : 'JavaScript'
  'text/x-jinja2'           : 'Jinja2'
  'application/json'        : 'JSON'
  'application/x-jsp'       : 'JSP'
  'text/x-less'             : 'LESS'
  'text/x-lua'              : 'Lua'
  'text/x-markdown'         : 'Markdown'
  'text/x-perl'             : 'Perl'
  'application/x-httpd-php' : 'PHP'
  'text/x-python'           : 'Python'
  'text/x-ruby'             : 'Ruby'
  'text/x-sass'             : 'Sass'
  'text/x-scala'            : 'Scala'
  'text/x-scheme'           : 'Scheme'
  'text/x-scss'             : 'SCSS'
  'text/x-sh'               : 'Shell Script'
  'text/x-sql'              : 'SQL'
  'application/xml'         : 'XML'
  'text/x-yaml'             : 'YAML'

(CodeMirror.MODES_ARRAY ||= []).push(mode: k, name: v) for k, v of CodeMirror.MODES

CodeMirror.MODES_WITH_LINTING =[
  'text/javascript'
  'application/json'
]

CodeMirror.commands.goMatchingBracket = (cm) ->
  match = cm.findMatchingBracket?()
  return false if !match
  # If cursor is outside of brackets, stay on the outside.
  # E.g. if cursor is before opening bracket, move to after the closing bracket.
  cur = cm.getCursor()
  if match.from.line < cur.line || (match.from.line == cur.line && match.from.ch < cur.ch)
    cm.extendSelection(match.to)
  else
    cm.extendSelection(line: match.to.line, ch: match.to.ch + 1)

CodeMirror.commands.unfold = (cm) ->
  cm.foldCode(cm.getCursor(), null, 'unfold');

CodeMirror.commands.insertLineBefore = (cm) ->
  cm.operation ->
    cm.execCommand('goLineStartSmart')
    cm.execCommand('newlineAndIndent')
    cm.setCursor(cm.getCursor().line - 1)
    cm.indentLine(cm.getCursor().line, null, true)

CodeMirror.commands.insertLineAfter = (cm) ->
  cm.operation ->
    cm.execCommand('goLineEnd')
    cm.replaceSelection("\n", 'end', '+input')
    cm.indentLine(cm.getCursor().line, null, true)

CodeMirror.commands.smartTab = (cm) ->
  # If there is a selection, pressing Tab will indent it, otherwise,
  # just insert a tab character or spaces.
  if cm.getSelection().length
    cm.execCommand('indentMore')
  else
    if cm.getOption('indentWithTabs')
      cm.execCommand('insertTab')
    else
      spaces = Array(cm.getOption('indentUnit') + 1).join(' ')
      cm.replaceSelection spaces, 'end', '+input'
