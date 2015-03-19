#= require polyfills
#= require webfonts
#= require ./lib/mp
#= require_self
#= require_tree ./pages

window.pages = {}
window.pages.activePageName = null

# Material buttons
# --------------------------------
materialBtns = true
if materialBtns
  ((window, $) ->
    $ ->
      $('.ripple').on 'click', (event) ->
        event.preventDefault()
        el = $(this)
        href = $(this).attr('href')
        $div = $('<div/>')
        btnOffset = $(this).offset()
        xPos = event.pageX - btnOffset.left
        yPos = event.pageY - btnOffset.top
        $div.addClass 'ripple-effect'
        $ripple = $('.ripple-effect')
        $ripple.css 'height', $(this).height()
        $ripple.css 'width', $(this).height()
        $div.css(
          top: yPos - ($ripple.height() / 2)
          left: xPos - ($ripple.width() / 2)
          background: $(this).data('ripple-color')).appendTo $(this)
        window.setTimeout (->
          $div.remove()
          if el.hasClass('submit')
            el.parents('form').submit()
            return false
          else
            if !(href == undefined)
              window.location = href
            return
        ), 985

        return
      return
    return
  ) window, jQuery

