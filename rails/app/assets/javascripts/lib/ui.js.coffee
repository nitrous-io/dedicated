$ = window.jQuery

window.ui =
  clearDropdowns: (options) ->
    $dropdowns = $('.ui.dropdown')
    if options?.except?
      $dropdowns = $dropdowns.not(options.except)
    $dropdowns.each (i, el) ->
      $el = $(el).removeClass('open')
      $menu = $el.find('ul.dropdown-menu')
      $menu.data('timeout', window.setTimeout(->
        $menu.hide()
      ,200))

  handleDropdowns: ->
    $('body').on 'click.ui.dropdown', '.ui.dropdown', (e) ->
      $dropdown = $(this)
      if $dropdown.hasClass('open')
        ui.clearDropdowns()
      else
        ui.clearDropdowns(except: $dropdown)
        $menu = $dropdown.find('ul.dropdown-menu').show()
        window.clearTimeout($menu.data('timeout'))
        $dropdown.addClass('open')

        $('body').one 'click.ui.dropdown', (e) ->
          if $(e.target).closest('.dropdown').length == 0
            ui.clearDropdowns()
            false
      false

  showModal: (jq) ->
    if !(jq instanceof jQuery)
      jq = $(jq)
    $modal = jq.show()
    window.setTimeout(->
      $modal.find('.focus').focus()
      $modal.addClass('show')
    , 1)

  hideModal: (jq) ->
    if !(jq instanceof jQuery)
      jq = $(jq)
    $modal = jq.removeClass('show')
    window.setTimeout(->
      $modal.hide()
    , 301)

$ ->
  ui.handleDropdowns()
