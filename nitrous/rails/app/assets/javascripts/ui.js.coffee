$ = window.jQuery

window.ui =
  clearDropdowns: (options) ->
    $dropdowns = $('.ui.dropdown')
    if options?.except?
      $dropdowns = $dropdowns.not(options.except)
    $dropdowns.each (i, el) ->
      $el = $(el).removeClass('open')
      $menu = $el.find('> ul.dropdown-menu')
      $menu.data('timeout', window.setTimeout(->
        $menu.hide()
      ,200))

  openDropdown: ($dropdown) ->
    if $dropdown.hasClass('open')
      $('body').off 'click.ui-dropdown-bg'
      ui.clearDropdowns()
    else
      ui.clearDropdowns(except: $dropdown)
      $menu = $dropdown.find('> ul.dropdown-menu').show()
      window.clearTimeout($menu.data('timeout'))
      $dropdown.addClass('open')

      $('body').one 'click.ui-dropdown-bg', (e) ->
        if $(e.target).closest('.dropdown').length == 0
          ui.clearDropdowns()
          false
    false

  handleDropdowns: ->
    $('body').on 'click.ui-dropdown', '.ui.dropdown', (e) ->
      ui.openDropdown($(this))

  showModal: ($modal, cb) ->
    if !($modal instanceof jQuery)
      $modal = $($modal)
    $modal.show()
    $modal.find('.focus').focus()
    window.setTimeout(->
      $modal.addClass('show')
      cb?()
    , 50)

  hideModal: ($modal, cb) ->
    if !($modal instanceof jQuery)
      $modal = $($modal)
    $modal.removeClass('show')
    window.setTimeout(->
      $modal.hide()
      cb?()
    , 301)

$ ->
  ui.handleDropdowns()
