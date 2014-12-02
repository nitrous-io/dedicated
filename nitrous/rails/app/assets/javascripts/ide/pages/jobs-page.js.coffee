#= require jquery
window.App ||= {}

App.JobsPage =
  initialize: ->
    chooseJob = ->
      if (hash = window.location.hash) != ''
        $expandable = $ "#{hash} .expandable"
        $expandable.next().addClass('expanded')
        $('a.more-details i', $expandable).addClass('fa-minus')

    $(window).on 'load', chooseJob
    $(window).on 'hashchange', chooseJob
    $('#position_links a').on 'click', chooseJob

    $('.expandable').click ->
      $this = $(this)
      expanded = $this.next().hasClass 'expanded'

      $('ul#jobs>li>.details').removeClass 'expanded'
      $('ul#jobs a.more-details i').removeClass 'fa-minus'

      unless expanded
        $this.next().addClass 'expanded'
        $('a.more-details i', $this).addClass 'fa-minus'

