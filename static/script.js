'use strict';
import * as ws from './ws.js';

function current_target() {
    return document.querySelector('button.list-group-item.active');
}

function htmlToElement(html) {
    const template = document.createElement('template');
    template.innerHTML = html;
    return template.content.firstChild;
}

document.addEventListener('DOMContentLoaded', function () {
    let last_target;

    const contacts = document.getElementById('contacts');
    const chat = document.getElementById('chat');
    const history = document.getElementById('history');
    const chat_title = document.querySelector('div.card-header > h3');
    const alert_container = document.querySelector('div.alert-container');

    const status_modal = document.getElementById('status');
    const modal_comp = new bootstrap.Modal(status_modal);

    const address_field = document.getElementById('nymo-address');
    const servers_list = document.getElementById('servers');
    const peers_list = document.getElementById('peers');
    const version_text = status_modal.getElementsByClassName('text-center')[0];

    function create_alert(content, timeout = 3000) {
        const alert = htmlToElement(`<div class="alert alert-danger alert-dismissible fade show" role="alert">
${content}<button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button></div>`);
        alert_container.prepend(alert);
        setTimeout(new bootstrap.Alert(alert).close, timeout);
    }

    ws.register('err', create_alert);

    ws.register('history', function ({id, content}) {
        if (current_target()?.dataset.id == id)
            history.innerHTML = content;
    });

    ws.register('new_msg', function ({target, message, content}) {
        const ele = contacts.querySelector(`button.list-group-item[data-id="${target}"]`);
        if (ele.classList.contains('active'))
            history.insertAdjacentHTML('afterbegin', content);
        if (message) {
            ele.dataset.message = message;
            update_name(ele);
        }
        contacts.prepend(ele);
    });

    ws.register('msg_sent', function (data) {
        if (data.err) {
            create_alert(data.err);
        }
        const ele = current_target();
        if (ele?.dataset.id != data.target) return;
        history.querySelector(`div.justify-content-end[data-id="${data.id}"]`)?.remove();
        if (data.content)
            history.insertAdjacentHTML('afterbegin', data.content);
        if (data.message) {
            ele.dataset.message = "You: " + data.message;
            update_name(ele);
        }
    });

    ws.register('meta', function ({version, address, servers, peers}) {
        version_text.innerText = version;
        address_field.innerText = address;
        address_field.href = address;
        servers_list.innerHTML = '';
        servers?.forEach(e => {
            const li = document.createElement('li');
            li.classList.add('list-group-item');
            li.innerText = e;
            servers_list.append(li);
        });
        peers_list.innerHTML = '';
        peers?.forEach(e => {
            const li = document.createElement('li');
            li.classList.add('list-group-item');
            li.innerText = e;
            peers_list.append(li);
        });
        modal_comp.show();
    });

    function update_name(btn) {
        if (btn.dataset.alias) {
            const ele = document.createElement('b');
            ele.innerText = btn.dataset.alias;
            btn.innerText = ` (${btn.dataset.addr})`;
            btn.insertAdjacentElement('afterbegin', ele);
        } else {
            btn.innerText = `(${btn.dataset.addr})`;
        }
        if (btn.dataset.message) {
            const node = document.createElement('p');
            node.className = 'm-0 small';
            node.innerText = btn.dataset.message;
            btn.insertAdjacentElement('beforeend', node);
        }
    }

    function update_title(btn) {
        if (btn.dataset.alias) {
            chat_title.innerHTML = ` <small class='text-muted'>(${btn.dataset.addr})</small>`;
            chat_title.insertAdjacentText('afterbegin', btn.dataset.alias);
        } else {
            chat_title.innerHTML = `<small>(${btn.dataset.addr})</small>`;
        }
    }

    function listen_button(btn) {
        update_name(btn);

        btn.addEventListener('click', function () {
            const current = current_target();
            if (current === this) return;
            if (current) current.classList.remove('active');
            else chat.style.removeProperty('display');
            history.innerHTML = '';
            ws.send('history', parseInt(this.dataset.id));
            this.classList.add('active');
            update_title(this);
        });
    }

    ws.register('new_user', function (content) {
        const button = htmlToElement(content);
        listen_button(button);
        contacts.prepend(button);
        if (button.dataset.addr === last_target) {
            last_target = undefined;
            button.click();
        }
    });

    for (const item of contacts.getElementsByClassName('list-group-item')) {
        listen_button(item);
    }

    const chat_input = document.getElementById('chat-input');
    chat_input.addEventListener('input', function () {
        if (this.value === '') {
            this.style.height = '1em';
        } else {
            this.style.height = 'auto';
            this.style.height = (this.scrollHeight) + 'px';
        }
    });

    document.getElementById('chat-send').addEventListener('click', function () {
        const val = chat_input.value;
        if (!val) return;
        let target;
        const current = current_target();
        if (current) {
            target = parseInt(current.dataset.id);
        } else {
            const input = chat_title.firstChild;
            if (input?.tagName !== 'INPUT') return;
            target = input.value.trim();
            input.value = '';
            const ele = contacts.querySelector(`button.list-group-item[data-addr="${target}"]`);
            if (ele) ele.click();
            else last_target = target;
        }
        chat_input.value = '';
        chat_input.style.height = '1em';
        ws.send('new_msg', {target: target, message: val});
    });

    document.getElementById('info-btn').addEventListener('click', function () {
        ws.send('meta');
    });

    document.getElementById('add-btn').addEventListener('click', function () {
        current_target()?.classList.remove('active');
        chat.style.removeProperty('display');
        chat_title.innerHTML = '<input type="text" class="form-control" placeholder="Address&hellip;">';
        history.innerHTML = '';
    });

    ws.register('alias', function (data) {
        const ele = contacts.querySelector(`button.list-group-item[data-id="${data.id}"]`);
        if (!ele) return;

        if (!data.name) delete ele.dataset.alias;
        else ele.dataset.alias = data.name;
        update_name(ele);
        if (ele.classList.contains('active'))
            update_title(ele);
    });

    chat_title.addEventListener('click', function () {
        const current = current_target();
        if (!current || this.firstChild.tagName === 'INPUT') return;
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'form-control';
        input.placeholder = 'Set Alias\u2026';
        input.addEventListener('keyup', function ({key}) {
            if (key === 'Enter') this.blur();
        });
        input.addEventListener('blur', function () {
            let name = this.value.trim();
            name = name === '' ? undefined : name;

            if (name === current.dataset.alias) {
                if (current.dataset.alias) this.replaceWith(current.dataset.alias + ' ');
                else this.remove();
                return;
            }
            ws.send('alias', {
                id: parseInt(current.dataset.id),
                name: name,
            });
        })
        if (current.dataset.alias) {
            input.value = current.dataset.alias;
            this.firstChild.replaceWith(input);
        } else {
            this.insertAdjacentElement('afterbegin', input);
        }
        input.focus();
    })
});