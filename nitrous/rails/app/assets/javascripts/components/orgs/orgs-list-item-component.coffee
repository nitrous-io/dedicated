App.OrgsListItemComponent = Em.Component.extend
  layoutName: 'components/orgs/list-item'
  tagName: 'li'
  classNames: ['orgs-list-item']
  classNameBindings: ['selected']

  item: null
  selectedItem: null

  selected: (->
    @get('item') == @get('selectedItem')
  ).property 'item', 'selectedItem'
