App.OrgsListItemComponent = Em.Component.extend
  tagName: 'li'
  templateName: 'components/orgs/list-item'
  classNames: ['orgs-list-item']
  classNameBindings: ['selected']

  item: null
  selectedItem: null

  selected: (->
    @get('item') == @get('selectedItem')
  ).property 'item', 'selectedItem'
