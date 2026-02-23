#ifndef TREE_SITTER_FSHARP_SCANNER_H_
#define TREE_SITTER_FSHARP_SCANNER_H_

#include "tree_sitter/alloc.h"
#include "tree_sitter/array.h"
#include "tree_sitter/parser.h"

enum TokenType {
  NEWLINE,
  INDENT,
  DEDENT,
  THEN,
  ELSE,
  ELIF,
  PREPROC_IF,
  PREPROC_ELSE,
  PREPROC_END,
  CLASS,
  STRUCT,
  INTERFACE,
  END,
  AND,
  TRIPLE_QUOTE_CONTENT,
  BLOCK_COMMENT_CONTENT,
  INSIDE_STRING,
  NEWLINE_NO_ALIGNED,
  TUPLE_MARKER,
  ERROR_SENTINEL
};

typedef struct {
  Array(uint16_t) indents;
  Array(uint16_t) preprocessor_indents;
} Scanner;

static inline void advance(TSLexer *lexer) { lexer->advance(lexer, false); }

static inline void skip(TSLexer *lexer) { lexer->advance(lexer, true); }

static inline bool scan_block_comment(TSLexer *lexer) {
  lexer->mark_end(lexer);
  if (lexer->lookahead != '(')
    return false;

  advance(lexer);
  if (lexer->lookahead != '*')
    return false;

  advance(lexer);

  while (true) {
    switch (lexer->lookahead) {
    case '(':
      scan_block_comment(lexer);
      break;
    case '*':
      advance(lexer);
      if (lexer->lookahead == ')') {
        advance(lexer);
        return true;
      }
      break;
    case '\0':
      return true;
    default:
      advance(lexer);
    }
  }
}

static inline bool is_infix_op_start(TSLexer *lexer) {
  switch (lexer->lookahead) {
  case '+':
    skip(lexer);
    return lexer->lookahead != '0' && lexer->lookahead != '1' &&
           lexer->lookahead != '2' && lexer->lookahead != '3' &&
           lexer->lookahead != '4' && lexer->lookahead != '5' &&
           lexer->lookahead != '6' && lexer->lookahead != '7' &&
           lexer->lookahead != '8' && lexer->lookahead != '9';
  case '-':
    skip(lexer);
    return lexer->lookahead != '0' && lexer->lookahead != '1' &&
           lexer->lookahead != '2' && lexer->lookahead != '3' &&
           lexer->lookahead != '4' && lexer->lookahead != '5' &&
           lexer->lookahead != '6' && lexer->lookahead != '7' &&
           lexer->lookahead != '8' && lexer->lookahead != '9';
  case '%':
  case '&':
  case '=':
  case '?':
  case '<':
  case '>':
  case '^':
    return true;
  case '/':
    skip(lexer);
    return lexer->lookahead != '/';
  case '.':
    skip(lexer);
    return lexer->lookahead != '.';
  case '!':
    skip(lexer);
    return lexer->lookahead == '=';
  case ':':
    skip(lexer);
    return lexer->lookahead == '=' || lexer->lookahead == ':' ||
           lexer->lookahead == '?' || lexer->lookahead == ' ' ||
           lexer->lookahead == '>';
  case 'o':
    skip(lexer);
    return lexer->lookahead == 'r';
  case '@':
  case '$':
    skip(lexer);
    return lexer->lookahead != '"';
  default:
    return false;
  }
}

static inline bool is_bracket_end(TSLexer *lexer) {
  switch (lexer->lookahead) {
  case ')':
  case ']':
  case '}':
    return true;
  default:
    return false;
  }
}

static bool scan(Scanner *scanner, TSLexer *lexer, const bool *valid_symbols) {
  if (valid_symbols[ERROR_SENTINEL]) {
    if (scanner->indents.size > 1) {
      array_pop(&scanner->indents);
      lexer->result_symbol = DEDENT;
      return true;
    }

    if (scanner->preprocessor_indents.size > 0) {
      array_pop(&scanner->preprocessor_indents);
      lexer->result_symbol = PREPROC_END;
      return true;
    }

    return false;
  }

  if (valid_symbols[INSIDE_STRING]) {
    return false;
  }

  if (valid_symbols[TRIPLE_QUOTE_CONTENT]) {
    lexer->mark_end(lexer);
    while (true) {
      if (lexer->lookahead == '\0') {
        break;
      }
      if (lexer->lookahead != '"') {
        advance(lexer);
      } else {
        lexer->mark_end(lexer);
        skip(lexer);
        if (lexer->lookahead == '"') {
          skip(lexer);
          if (lexer->lookahead == '"') {
            skip(lexer);
            break;
          }
        }
        lexer->mark_end(lexer);
      }
    }
    lexer->result_symbol = TRIPLE_QUOTE_CONTENT;
    return true;
  }

  lexer->mark_end(lexer);

  bool found_end_of_line = false;
  bool found_start_of_infix_op = false;
  bool found_bracket_end = false;
  bool found_preprocessor_end = false;
  bool found_comment = false;
  uint32_t indent_length = lexer->get_column(lexer);

  for (;;) {
    if (lexer->lookahead == '\n') {
      found_end_of_line = true;
      indent_length = 0;
      skip(lexer);
    } else if (lexer->lookahead == ' ') {
      indent_length++;
      skip(lexer);
    } else if (lexer->lookahead == '\r' || lexer->lookahead == '\f') {
      indent_length = 0;
      skip(lexer);
    } else if (lexer->lookahead == '\t') {
      indent_length += 8;
      skip(lexer);
    } else if (lexer->eof(lexer)) {
      found_end_of_line = true;
      break;
    } else if (lexer->lookahead == '/') {
      skip(lexer);
      if (!valid_symbols[INSIDE_STRING] && lexer->lookahead == '/') {
        found_comment = true;
        while (lexer->lookahead != '\n' && !lexer->eof(lexer)) {
          skip(lexer);
        }
      } else {
        return false;
      }
    } else if (lexer->lookahead == '#' && indent_length == 0) {
      advance(lexer);
      if (lexer->lookahead == 'e') {
        advance(lexer);
        if (lexer->lookahead == 'n') {
          advance(lexer);
          if (lexer->lookahead == 'd') {
            advance(lexer);
            if (lexer->lookahead == 'i') {
              advance(lexer);
              if (lexer->lookahead == 'f') {
                advance(lexer);
                found_preprocessor_end = true;
                if (scanner->indents.size > 0 &&
                    scanner->preprocessor_indents.size > 0) {
                  uint16_t current_indent_length =
                      *array_back(&scanner->indents);
                  uint16_t current_preproc_length =
                      *array_back(&scanner->preprocessor_indents);
                  if (current_preproc_length < current_indent_length) {
                    array_pop(&scanner->indents);
                    lexer->result_symbol = DEDENT;
                    return true;
                  }
                }
                if (valid_symbols[PREPROC_END] && !found_comment) {
                  if (scanner->preprocessor_indents.size > 0) {
                    array_pop(&scanner->preprocessor_indents);
                  }
                  lexer->mark_end(lexer);
                  lexer->result_symbol = PREPROC_END;
                  return true;
                }
              }
            }
          }
        } else if (lexer->lookahead == 'l') {
          advance(lexer);
          if (lexer->lookahead == 's') {
            advance(lexer);
            if (lexer->lookahead == 'e') {
              advance(lexer);
              if (scanner->indents.size > 0 &&
                  scanner->preprocessor_indents.size > 0) {
                uint16_t current_indent_length = *array_back(&scanner->indents);
                uint16_t current_preproc_length =
                    *array_back(&scanner->preprocessor_indents);
                if (current_preproc_length < current_indent_length) {
                  array_pop(&scanner->indents);
                  lexer->result_symbol = DEDENT;
                  return true;
                }
              }
              if (valid_symbols[PREPROC_ELSE] && !found_comment) {
                lexer->mark_end(lexer);
                lexer->result_symbol = PREPROC_ELSE;
                return true;
              }
            }
          }
        }
      } else if (lexer->lookahead == 'i') {
        advance(lexer);
        if (lexer->lookahead == 'f') {
          advance(lexer);
          if (valid_symbols[NEWLINE] || valid_symbols[INDENT]) {
            while (lexer->lookahead != '\n' && !lexer->eof(lexer)) {
              skip(lexer);
            }
          } else {
            if (scanner->indents.size > 0) {
              if (valid_symbols[PREPROC_IF]) {
                uint16_t current_indent_length = *array_back(&scanner->indents);
                array_push(&scanner->preprocessor_indents,
                           current_indent_length);
              } else {
                array_pop(&scanner->indents);
                lexer->result_symbol = DEDENT;
                return true;
              }
            } else if (!found_comment) {
              lexer->mark_end(lexer);
              lexer->result_symbol = PREPROC_IF;
              return true;
            }
          }
        }
      } else {
        if (found_end_of_line && valid_symbols[NEWLINE_NO_ALIGNED]) {
          lexer->result_symbol = NEWLINE_NO_ALIGNED;
          return true;
        }
        return false;
      }
    } else {
      break;
    }
  }

  if (valid_symbols[CLASS] && lexer->lookahead == 'c') {
    lexer->mark_end(lexer);
    indent_length = lexer->get_column(lexer);
    advance(lexer);
    if (lexer->lookahead == 'l') {
      advance(lexer);
      if (lexer->lookahead == 'a') {
        advance(lexer);
        if (lexer->lookahead == 's') {
          advance(lexer);
          if (lexer->lookahead == 's') {
            advance(lexer);
            lexer->mark_end(lexer);
            lexer->result_symbol = CLASS;
            return true;
          }
        }
      }
    }
  } else if (valid_symbols[STRUCT] && lexer->lookahead == 's') {
    lexer->mark_end(lexer);
    indent_length = lexer->get_column(lexer);
    advance(lexer);
    if (lexer->lookahead == 't') {
      advance(lexer);
      if (lexer->lookahead == 'r') {
        advance(lexer);
        if (lexer->lookahead == 'u') {
          advance(lexer);
          if (lexer->lookahead == 'c') {
            advance(lexer);
            if (lexer->lookahead == 't') {
              advance(lexer);
              lexer->mark_end(lexer);
              lexer->result_symbol = STRUCT;
              return true;
            }
          }
        }
      }
    }
  } else if (valid_symbols[INTERFACE] && lexer->lookahead == 'i') {
    lexer->mark_end(lexer);
    indent_length = lexer->get_column(lexer);
    advance(lexer);
    if (lexer->lookahead == 'n') {
      advance(lexer);
      if (lexer->lookahead == 't') {
        advance(lexer);
        if (lexer->lookahead == 'e') {
          advance(lexer);
          if (lexer->lookahead == 'r') {
            advance(lexer);
            if (lexer->lookahead == 'f') {
              advance(lexer);
              if (lexer->lookahead == 'a') {
                advance(lexer);
                if (lexer->lookahead == 'c') {
                  advance(lexer);
                  if (lexer->lookahead == 'e') {
                    advance(lexer);
                    lexer->mark_end(lexer);
                    lexer->result_symbol = INTERFACE;
                    return true;
                  }
                }
              }
            }
          }
        }
      }
    }
  }

  if (found_end_of_line && valid_symbols[NEWLINE_NO_ALIGNED] &&
      !found_start_of_infix_op && !found_preprocessor_end) {
    lexer->result_symbol = NEWLINE_NO_ALIGNED;
    return true;
  }

  if (valid_symbols[NEWLINE] && lexer->lookahead == ';') {
    advance(lexer);
    lexer->mark_end(lexer);
    lexer->result_symbol = NEWLINE;
    return true;
  }

  if (lexer->lookahead == 't' &&
      (valid_symbols[THEN] || valid_symbols[DEDENT])) {
    advance(lexer);
    if (lexer->lookahead == 'h') {
      advance(lexer);
      if (lexer->lookahead == 'e') {
        advance(lexer);
        if (lexer->lookahead == 'n') {
          advance(lexer);
          // the 'THEN' token is only valid if we have popped the appropriate
          // amount of dedent tokens.
          // If 'THEN' is not valid we just continue to pop dedent tokens.
          if (valid_symbols[THEN]) {
            lexer->mark_end(lexer);
            lexer->result_symbol = THEN;
            return true;
          } else {
            array_pop(&scanner->indents);
            lexer->result_symbol = DEDENT;
            return true;
          }
        }
      }
    }
  } else if (lexer->lookahead == 'a' && valid_symbols[AND] && !found_comment) {
    advance(lexer);
    if (lexer->lookahead == 'n') {
      advance(lexer);
      if (lexer->lookahead == 'd') {
        advance(lexer);
        if (lexer->lookahead == ' ') {
          lexer->result_symbol = AND;
          lexer->mark_end(lexer);
          return true;
        }
      }
    }
  } else if (lexer->lookahead == 'e' &&
             (valid_symbols[ELSE] || valid_symbols[ELIF] ||
              valid_symbols[END] || valid_symbols[DEDENT]) &&
             !found_comment) {
    advance(lexer);
    int16_t token_indent_level = lexer->get_column(lexer);
    if (lexer->lookahead == 'l') {
      advance(lexer);
      if (lexer->lookahead == 's' &&
          (valid_symbols[ELSE] || valid_symbols[DEDENT])) {
        advance(lexer);
        if (lexer->lookahead == 'e') {
          advance(lexer);
          if (valid_symbols[ELSE]) {
            if (scanner->indents.size > 0 &&
                token_indent_level < *array_back(&scanner->indents)) {
              array_pop(&scanner->indents);
              lexer->result_symbol = DEDENT;
              return true;
            } else {
              lexer->mark_end(lexer);
              for (;;) {
                if (lexer->lookahead == ' ' || lexer->lookahead == '\n' ||
                    lexer->lookahead == '\r' || lexer->lookahead == '\t') {
                  advance(lexer);
                } else {
                  break;
                }
              }
              if (lexer->lookahead == 'i') {
                advance(lexer);
                if (lexer->lookahead == 'f') {
                  advance(lexer);
                  if (lexer->lookahead == ' ' || lexer->lookahead == '\n' ||
                      lexer->lookahead == '\t') {
                    lexer->mark_end(lexer);
                    lexer->result_symbol = ELIF;
                    return true;
                  }
                }
              }
              lexer->result_symbol = ELSE;
              return true;
            }
          } else {
            array_pop(&scanner->indents);
            lexer->result_symbol = DEDENT;
            return true;
          }
        }
      } else if (lexer->lookahead == 'i' &&
                 (valid_symbols[ELIF] || valid_symbols[DEDENT])) {
        advance(lexer);
        if (lexer->lookahead == 'f') {
          advance(lexer);
          if (valid_symbols[ELIF]) {
            if (scanner->indents.size > 0 &&
                token_indent_level < *array_back(&scanner->indents)) {
              array_pop(&scanner->indents);
              lexer->result_symbol = DEDENT;
              return true;
            } else {
              lexer->mark_end(lexer);
              lexer->result_symbol = ELIF;
              return true;
            }
          } else {
            array_pop(&scanner->indents);
            lexer->result_symbol = DEDENT;
            return true;
          }
        }
      }
    } else if (lexer->lookahead == 'n' &&
               (valid_symbols[END] || valid_symbols[DEDENT])) {
      advance(lexer);
      if (lexer->lookahead == 'd') {
        advance(lexer);
        if (lexer->lookahead == ' ' || lexer->lookahead == '\n' ||
            lexer->eof(lexer)) {
          if (valid_symbols[END]) {
            lexer->mark_end(lexer);
            lexer->result_symbol = END;
            return true;
          } else if (valid_symbols[DEDENT] && scanner->indents.size > 0) {
            array_pop(&scanner->indents);
            lexer->result_symbol = DEDENT;
            return true;
          }
        }
      }
    }
  } else if (is_bracket_end(lexer)) {
    found_bracket_end = true;
  } else if (is_infix_op_start(lexer)) {
    found_start_of_infix_op = true;
  } else if (lexer->lookahead == '|') {
    skip(lexer);
    switch (lexer->lookahead) {
    case ']':
    case '}':
      found_bracket_end = true;
      break;
    case ' ':
      if (scanner->indents.size > 0) {
        uint16_t current_indent_length = *array_back(&scanner->indents);
        if (found_end_of_line && indent_length == current_indent_length &&
            indent_length > 0 && !found_start_of_infix_op &&
            !found_bracket_end) {
          if (valid_symbols[NEWLINE] && !found_preprocessor_end) {
            lexer->result_symbol = NEWLINE;
            return true;
          }
        }
      }
      break;
    default:
      found_start_of_infix_op = true;
      break;
    }
  }

  if (valid_symbols[INDENT] && !found_bracket_end && !found_preprocessor_end) {
    array_push(&scanner->indents, indent_length);
    lexer->result_symbol = INDENT;
    return true;
  }

  if (scanner->indents.size > 0) {
    uint16_t current_indent_length = *array_back(&scanner->indents);

    if (found_bracket_end && valid_symbols[DEDENT]) {
      array_pop(&scanner->indents);
      lexer->result_symbol = DEDENT;
      return true;
    }

    if (found_end_of_line) {
      if (indent_length == current_indent_length && indent_length > 0 &&
          !found_start_of_infix_op && !found_bracket_end) {
        if (valid_symbols[NEWLINE] && !found_preprocessor_end) {
          lexer->result_symbol = NEWLINE;
          return true;
        }
      }

      bool can_dedent_preproc;

      if (scanner->preprocessor_indents.size > 0) {
        uint16_t current_preproc_length =
            *array_back(&scanner->preprocessor_indents);
        can_dedent_preproc = current_preproc_length < indent_length;
      } else {
        can_dedent_preproc = true;
      }

      bool can_dedent_infix_op;

      if (found_start_of_infix_op) {
        can_dedent_infix_op = indent_length + 1 < current_indent_length;
      } else {
        can_dedent_infix_op = true;
      }

      if (indent_length < current_indent_length && !found_bracket_end &&
          can_dedent_preproc && can_dedent_infix_op &&
          !valid_symbols[TUPLE_MARKER]) {
        array_pop(&scanner->indents);
        lexer->result_symbol = DEDENT;
        return true;
      }
    }
  }

  if (valid_symbols[BLOCK_COMMENT_CONTENT]) {
    lexer->mark_end(lexer);
    while (true) {
      if (lexer->lookahead == '\0') {
        break;
      }
      if (lexer->lookahead != '(' && lexer->lookahead != '*') {
        advance(lexer);
      } else if (lexer->lookahead == '*') {
        lexer->mark_end(lexer);
        advance(lexer);
        if (lexer->lookahead == ')') {
          break;
        }
      } else if (scan_block_comment(lexer)) {
        lexer->mark_end(lexer);
        advance(lexer);
        if (lexer->lookahead == '*') {
          break;
        }
      }
    }
    lexer->result_symbol = BLOCK_COMMENT_CONTENT;
    return true;
  }

  return false;
}

static unsigned serialize(Scanner *scanner, char *buffer) {
  size_t size = 0;

  size_t preprocessor_count = scanner->preprocessor_indents.size;
  if (preprocessor_count > UINT8_MAX) {
    preprocessor_count = UINT8_MAX;
  }

  buffer[size++] = (char)scanner->preprocessor_indents.size;

  for (size_t iter = 0; iter < preprocessor_count &&
                        size < TREE_SITTER_SERIALIZATION_BUFFER_SIZE;
       iter++) {
    char e = *array_get(&scanner->preprocessor_indents, iter);
    buffer[size++] = e;
  }

  uint32_t iter = 1;
  for (; iter < scanner->indents.size &&
         size < TREE_SITTER_SERIALIZATION_BUFFER_SIZE;
       ++iter) {
    buffer[size++] = (char)*array_get(&scanner->indents, iter);
  }

  return size;
}

static void deserialize(Scanner *scanner, const char *buffer, unsigned length) {
  array_delete(&scanner->indents);
  array_push(&scanner->indents, 0);

  array_delete(&scanner->preprocessor_indents);
  if (length > 0) {
    size_t size = 0;
    size_t preprocessor_count = (uint8_t)buffer[size++];

    for (; size <= preprocessor_count; size++) {
      array_push(&scanner->preprocessor_indents, (unsigned char)buffer[size]);
    }

    for (; size < length; size++) {
      array_push(&scanner->indents, (unsigned char)buffer[size]);
    }

    assert(size == length);
  }
}

static Scanner *create() {
  Scanner *scanner = ts_calloc(1, sizeof(Scanner));
  array_init(&scanner->indents);
  array_init(&scanner->preprocessor_indents);
  deserialize(scanner, NULL, 0);
  return scanner;
}

static void destroy(Scanner *scanner) {
  array_delete(&scanner->indents);
  array_delete(&scanner->preprocessor_indents);
  ts_free(scanner);
}

#endif // TREE_SITTER_FSHARP_SCANNER_H_
