$ = window.jQuery

clearDropdowns = (options) ->
  $dropdowns = $('.ui.dropdown')
  if options?.except?
    $dropdowns = $dropdowns.not(options.except)
  $dropdowns.removeClass('open')

handleDropdowns = ->
  $('body').on 'click.ui.dropdown', '.ui.dropdown', (e) ->
    $dropdown = $(this)
    clearDropdowns(except: $dropdown)
    $dropdown.toggleClass('open')

    if $dropdown.hasClass('open')
      $('body').one 'click.ui.dropdown', (e) ->
        if $(e.target).closest('.dropdown').length == 0
          clearDropdowns()
          false

$ ->
  handleDropdowns()

