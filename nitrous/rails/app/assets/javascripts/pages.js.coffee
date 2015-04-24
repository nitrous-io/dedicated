#= require polyfills
#= require webfonts
#= require ./lib/mp
#= require_self
#= require_tree ./pages

window.pages = {}
window.pages.activePageName = null
  
if $("body").hasClass("stacks-home")

  $("body").append('<style id="search-css"></style>')
  
  $('#stacks-search').keyup ->
    if !@value
      $("#search-css").html('')
      return
    $("#search-css").html('.searchable:not([data-index*="' + @value.toLowerCase() + '"]) { display: none; }')
    return