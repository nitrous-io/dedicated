#= require polyfills
#= require webfonts
#= require ./lib/mp
#= require_self
#= require_tree ./pages

window.pages = {}
window.pages.activePageName = null
  
# Stacks
# ------------------------------------
if $("body").hasClass("stacks-home")
  $("body").append('<style id="search-css"></style>')
  $('#stacks-search').keyup ->
    if !@value
      $("#search-css").html('')
      return
    $("#search-css").html('.searchable:not([data-index*="' + @value.toLowerCase() + '"]) { display: none; }')
    return


# Whatsnew
# ------------------------------------
if $("body").hasClass("introducing")
  parallax = ->
    scrolltop = window.pageYOffset
    parallaxEl.style.top = -scrolltop * .2 + 'px'
    return

  window.requestAnimationFrame = window.requestAnimationFrame or window.mozRequestAnimationFrame or window.webkitRequestAnimationFrame or window.msRequestAnimationFrame or (f) ->
    setTimeout f, 1000 / 60
    return
  parallaxEl = $(".parallaxEl")[0]
  window.addEventListener 'scroll', (->
    requestAnimationFrame parallax
    return
  ), false
