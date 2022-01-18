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

    function create_alert(content, timeout = 3000) {
        const alert = htmlToElement(`<div class="alert alert-danger alert-dismissible fade show" role="alert">
${content}<button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button></div>`);
        alert_container.prepend(alert);
        setTimeout(() => new bootstrap.Alert(alert).close(), timeout);
    }

    ws.register('err', create_alert);

    ws.register('history', function ({id, content}) {
        if (current_target()?.dataset.id == id)
            history.innerHTML = content;
    });

    ws.register('new_msg', function ({target, content}) {
        const ele = document.querySelector(`button.list-group-item[data-id="${target}"]`);
        if (ele.classList.contains('active'))
            history.insertAdjacentHTML('afterbegin', content);
        contacts.prepend(ele);
    });

    ws.register('msg_sent', function (data) {
        if (data.err) { // TODO
            console.error(data.err);
        }
        if (current_target()?.dataset.id != data.target) return;
        history.querySelector(`div.justify-content-end[data-id="${data.id}"]`)?.remove();
        if (data.content)
            history.insertAdjacentHTML('afterbegin', data.content);
    })

    function update_name(btn) {
        if (btn.dataset.alias) {
            btn.innerHTML = `<b>${btn.dataset.alias}</b> (${btn.dataset.addr})`;
        } else {
            btn.innerText = `(${btn.dataset.addr})`;
        }
    }

    function update_title(btn) {
        if (btn.dataset.alias) {
            chat_title.innerHTML = `${btn.dataset.alias} <small class='text-muted'>(${btn.dataset.addr})</small>`;
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
            const ele = document.querySelector(`button.list-group-item[data-addr="${target}"]`);
            if (ele) ele.click();
            else last_target = target;
        }
        chat_input.value = '';
        chat_input.style.height = '1em';
        ws.send('new_msg', {target: target, content: val});
    });

    document.getElementById('add-btn').addEventListener('click', function () {
        current_target()?.classList.remove('active');
        chat.style.removeProperty('display');
        chat_title.innerHTML = '<input type="text" class="form-control" placeholder="Address&hellip;">';
        history.innerHTML = '';
    });

    ws.register('alias', function (data) {
        const ele = document.querySelector(`button.list-group-item[data-id="${data.id}"]`);
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