;;; sample.el --- test fixture

(defun greet (name)
  "Say hello."
  (message "hi %s" name))

(defvar my-counter 0
  "Counter docstring.")

(defmacro when-not (cond &rest body)
  `(if (not ,cond) ,@body))

(defcustom user-name "alice"
  "The user's name."
  :type 'string)

(defconst max-retries 5 "Max retries.")

(defalias 'old-name #'greet)

(defface my-face '((t :foreground "red")) "")

(define-minor-mode my-mode "Toggle mode." :init-value nil)

(define-derived-mode my-major prog-mode "MyMaj" "Doc")

(cl-defmethod foo ((x string)) "method-impl")

(cl-defun fancy-fn (&key arg) (+ arg 1))
