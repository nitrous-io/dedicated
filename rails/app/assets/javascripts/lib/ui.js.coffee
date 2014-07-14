$ = window.jQuery

clearDropdowns = (options) ->
  $dropdowns = $('.ui.dropdown')
  if options?.except?
    $dropdowns = $dropdowns.not(options.except)
  $dropdowns.each (i, el) ->
    $el = $(el).removeClass('open')
    $menu = $el.find('ul.dropdown-menu')
    $menu.data('timeout', window.setTimeout(->
      $menu.hide()
    ,200))

handleDropdowns = ->
  $('body').on 'click.ui.dropdown', '.ui.dropdown', (e) ->
    $dropdown = $(this)
    if $dropdown.hasClass('open')
      clearDropdowns()
    else
      clearDropdowns(except: $dropdown)
      $menu = $dropdown.find('ul.dropdown-menu').show()
      window.clearTimeout($menu.data('timeout'))
      $dropdown.addClass('open')

      $('body').one 'click.ui.dropdown', (e) ->
        if $(e.target).closest('.dropdown').length == 0
          clearDropdowns()
          false
    false

$ ->
  handleDropdowns()

