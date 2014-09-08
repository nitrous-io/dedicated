#= require webfonts
#= require md5
#= require pointer_events_polyfill
#= require jquery.autosize
#= require jquery.timeago-1.4.1
#= require async-0.2.5
#= require codemirror
#= require lib/ui
#= require_self
#= require app

$ ->
  PointerEventsPolyfill.initialize(selector: '.click-thru')
