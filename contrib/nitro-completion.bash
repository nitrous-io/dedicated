#!/bin/bash

if ! type _get_comp_words_by_ref >/dev/null 2>&1; then
_get_comp_words_by_ref ()
{
  local exclude cur_ words_ cword_
  if [ "$1" = "-n" ]; then
    exclude=$2
    shift 2
  fi
  cur_=${words_[cword_]}
  while [ $# -gt 0 ]; do
    case "$1" in
    cur)
      cur=$cur_
      ;;
    prev)
      prev=${words_[$cword_-1]}
      ;;
    words)
      words=("${words_[@]}")
      ;;
    cword)
      cword=$cword_
      ;;
    esac
    shift
  done
}
fi

_get_pos_first_nonflag()
{
  local argument_flags=$1

  local counter=$cpos
  while [ $counter -le $cword ]; do
    if [ -n "$argument_flags" ] && eval "case '${words[$counter]}' in $argument_flags) true ;; *) false ;; esac"; then
      (( counter++ ))
    else
      case "${words[$counter]}" in
        -*)
          ;;
        *)
          break
          ;;
      esac
    fi
    (( counter++ ))
  done

  echo $counter
}

__nitro_q() {
  nitro 2>/dev/null "$@"
}

__nitro_hosts()
{
  local hosts="$( __nitro_q hosts | awk 'NR>1{print $1}' | grep -v '^<none>$' )"
  COMPREPLY=( $( compgen -W "$hosts" -- "$cur" ) )
}

__nitro_containers()
{
  local containers="$( __nitro_q list | awk 'NR>1{print $1}' | grep -v '^<none>$' )"
  COMPREPLY=( $( compgen -W "$containers" -- "$cur" ) )
}

__nitro_sizes()
{
  local sizes="$( __nitro_q plans | awk 'NR>1{print $1}' | grep -v '^<none>$' )"
  COMPREPLY=( $( compgen -W "$sizes" -- "$cur" ) )
}

__nitro_regions()
{
  local regions="$( __nitro_q regions | awk 'NR>1{print $1}' | grep -v '^<none>$' )"
  COMPREPLY=( $( compgen -W "$regions" -- "$cur" ) )
}

_nitro_nitro()
{
  case "$prev" in
    -H)
      return
      ;;
    *)
      ;;
  esac

  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-h --help" -- "$cur" ) )
      ;;
    *)
      COMPREPLY=( $( compgen -W "$commands help" -- "$cur" ) )
      ;;
  esac
}

_nitro_create()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-s --slug -i --image -e --env -v --volume -p --port-forward" -- "$cur" ) )
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_destroy()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_hosts()
{
  return
}

_nitro_hosts:create()
{
  case "$prev" in
    -r|--region)
      __nitro_regions
      return
      ;;
    *)
      __nitro_sizes
      ;;
  esac

  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-r --region -s --slug" -- "$cur" ) )
      ;;
    *)
      ;;
  esac
}

_nitro_hosts:destroy()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_hosts:start()
{
  case "$cur" in
    -*)
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_hosts:stop()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_hosts:restart()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_inspect()
{
  case "$cur" in
    -*)
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_keys()
{
  case "$cur" in
    -*)
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_keys:add()
{
  local counter=$(_get_pos_first_nonflag)
  if [ $cword -eq $counter ]; then
    case "$cur" in
      *:)
        return
        ;;
      *)
        __nitro_containers
        return
        ;;
    esac
  fi
  (( counter++ ))

  if [ $cword -eq $counter ]; then
    _filedir
    return
  fi
}

_nitro_keys:clear()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_keys:remove()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_list()
{
  case "$cur" in
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_logout()
{
  return
}

_nitro_registry:login()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-u --username -p --password -e --email -r --registry" -- "$cur" ) )
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_registry:login()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-r --registry" -- "$cur" ) )
      ;;
    *)
      __nitro_hosts
      return
      ;;
  esac
}

_nitro_restart()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_start()
{
  case "$cur" in
    -*)
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro_stop()
{
  case "$cur" in
    -*)
      COMPREPLY=( $( compgen -W "-f --force" -- "$cur" ) )
      ;;
    *)
      __nitro_containers
      return
      ;;
  esac
}

_nitro()
{
  local commands="
                  create
                  destroy
                  hosts
                  hosts:create
                  hosts:destroy
                  hosts:restart
                  hosts:start
                  hosts:stop
                  inspect
                  keys
                  keys:add
                  keys:clear
                  keys:remove
                  list
                  login
                  logout
                  registry:login
                  registry:logout
                  restart
                  start
                  stop
  "
  COMPREPLY=()
  local cur prev words cword
  _get_comp_words_by_ref -n : cur prev words cword

  local command='nitro'
  local counter=1
  while [ $counter -lt $cword ]; do
    case "${words[$counter]}" in
      -H)
        (( counter++ ))
        ;;
      -*)
        ;;
      *)
        command="${words[$counter]}"
        cpos=$counter
        (( cpos++ ))
        break
        ;;
    esac
    (( counter++ ))
  done

  local completions_func=_nitro_${command}
  declare -F $completions_func >/dev/null && $completions_func

  return 0
}

complete -F _nitro nitro
COMP_WORDBREAKS=${COMP_WORDBREAKS//:}
