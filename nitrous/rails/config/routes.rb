Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  get '/ide' => 'web_app#ide'
  get '/features' => 'pages#features'
  get '/pricing' => 'pages#pricing'
  get '/company' => 'pages#company'
  get '/plan_changes' => 'pages#plan_changes'


  get '/stacks' => 'pages#stacks'
  get '/stacks/:stack' => 'pages#stack'

  root 'pages#home'
end
