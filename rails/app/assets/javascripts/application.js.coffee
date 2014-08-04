#= require jquery
#= require webfonts
#= require md5
#= require pointer_events_polyfill
#= require jquery.autosize
#= require codemirror
#= require lib/ui
#= require_self
#= require app

$ ->
  PointerEventsPolyfill.initialize(selector: '.click-thru')
