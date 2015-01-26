pages.Home =
  screens: ['ide', 'main', 'desktop']
  page: 0
  interval: null

  init: ->
    $ =>
      @startTransitions()

    $('.pages .page').click (e) =>
      $page = $(e.target)
      @page = parseInt($page.attr('data-page'), 10)
      @startTransitions()
      false

    $("#get-started").submit (event) ->
      event.preventDefault()
      url = '/app/#/signup'
      if email = $(event.target).find("[name='email']").val()
        url += "?email=#{encodeURIComponent(email)}"
      # set href instead of replace to preserve history (navigate instead of redirect)
      window.location.href = url

  startTransitions: ->
    window.clearInterval(@interval)
    @interval = window.setInterval(=>
      @transition()
    , 10000)
    @transition()

  transition: ->
    screen = @screens[@page]
    $(".overview").addClass('fadeout')
    $(".overview.#{screen}").removeClass('fadeout')
    $(".pages .page").removeClass('active')
    $(".pages .page[data-page='#{@page}']").addClass('active')
    @page = (@page + 1) % 3
