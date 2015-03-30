App.InvoiceItemView = Em.View.extend
  tagName: 'tr'
  templateName: 'invoice-item'

  item: null

  actions:
    toggleItems: ->
      related = $(event.target).parents(".title").siblings(".items")
      if related.hasClass("show")
        related.removeClass("flow")
        related.removeClass("show")
      else
        related.addClass("show")
        window.setTimeout(->
          related.addClass("flow")
         , 500)