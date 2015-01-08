$ = window.jQuery

window.ui =
  clearDropdowns: (options) ->
    $('body').off 'click.ui-dropdown-bg'
    $dropdowns = $('.ui.dropdown')
    if options?.except?
      $dropdowns = $dropdowns.not(options.except)
    $dropdowns.each (i, el) ->
      $el = $(el).removeClass('open')
      $menu = $el.find('> ul.dropdown-menu')
      $menu.data('timeout', window.setTimeout(->
        $menu.hide()
      ,200))

  openDropdown: (evt, $dropdown) ->
    if $dropdown.hasClass('open')
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
    evt?.stopPropagation()
    evt?.preventDefault()

  handleDropdowns: ->
    $('body').on 'click.ui-dropdown-menuitem', '.ui.dropdown ul.dropdown-menu > li a', (evt) ->
      ui.clearDropdowns()
      evt.stopPropagation()
      href = $(this).attr('href')
      if !href || href == '#'
        evt.preventDefault()

    $('body').on 'click.ui-dropdown', '.ui.dropdown', (evt) ->
      ui.openDropdown(evt, $(this))

  showModal: ($modal, cb) ->
    if !($modal instanceof jQuery)
      $modal = $($modal)
    if window.util.IsSafari
      $modal.css(display: '-webkit-flex')
    else
      $modal.css(display: 'flex')
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
      $modal.css(display: 'none')
      cb?()
    , 301)

$ ->
  ui.handleDropdowns()
