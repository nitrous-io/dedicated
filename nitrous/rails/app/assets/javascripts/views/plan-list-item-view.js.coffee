App.PlanListItemView = Em.View.extend
  templateName: 'plan-list-item'
  tagName: 'li'
  classNames: ['plan-list-item-view']
  classNameBindings: ['isSelected:selected']

  content: null
  selected: null

  isSelected: (->
    @get('selected') == @get('content')
  ).property 'selected', 'content'

  click: ->
    @set('selected', @get('content'))
