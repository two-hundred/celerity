use std::fmt;

use serde_json::Value;

/// The error type used for template function
/// call errors.
#[derive(Debug)]
pub enum FunctionCallError {
    InvalidArgument(String),
    IncorrectNumberOfArguments(String),
}

impl fmt::Display for FunctionCallError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            FunctionCallError::InvalidArgument(arg) => {
                write!(f, "function call error: invalid argument: {}", arg)
            }
            FunctionCallError::IncorrectNumberOfArguments(func) => {
                write!(
                    f,
                    "function call error: incorrect number of arguments: {}",
                    func
                )
            }
        }
    }
}

/// V1 Workflow Template Function `format` implementation.
///
/// This function formats a string using the provided arguments.
/// The use of `{}` in the format string will be replaced by the arguments
/// in the order they are provided.
///
/// See [format function definition](docs/applications/resources/celerity-workflow#format).
pub fn format(args: Vec<Value>) -> Result<Value, FunctionCallError> {
    if args.len() < 1 {
        return Err(FunctionCallError::IncorrectNumberOfArguments(
            "format function requires at least one argument".to_string(),
        ));
    }

    let format_string = match &args[0] {
        Value::String(string) => string,
        _ => {
            return Err(FunctionCallError::InvalidArgument(
                "format function requires the first argument to be a string".to_string(),
            ))
        }
    };

    let placeholder_count = format_string.matches("{}").count();
    if args.len() - 1 != placeholder_count as usize {
        return Err(FunctionCallError::IncorrectNumberOfArguments(format!(
            "format function requires {} arguments after the format string, \
            one for each \"{{}}\" placeholder",
            placeholder_count
        )));
    }

    let mut formatted = format_string.to_string();
    for arg in args.iter().skip(1) {
        match arg {
            Value::String(string) => {
                formatted = formatted.replacen("{}", string, 1);
            }
            Value::Number(number) => {
                formatted = formatted.replacen("{}", &number.to_string(), 1);
            }
            Value::Bool(boolean) => {
                formatted = formatted.replacen("{}", &boolean.to_string(), 1);
            }
            Value::Null => {
                formatted = formatted.replacen("{}", "null", 1);
            }
            Value::Array(_) | Value::Object(_) => {
                return Err(FunctionCallError::InvalidArgument(
                    "format function does not support arrays or objects as arguments".to_string(),
                ));
            }
        }
    }
    Ok(Value::String(formatted))
}

#[cfg(test)]
mod format_tests {
    use super::*;
    use serde_json::json;

    #[test_log::test]
    fn test_format_simple() {
        let args = vec![json!("This is a simple {}!"), json!("test")];
        let result = format(args).unwrap();
        assert_eq!(result, json!("This is a simple test!"));
    }

    #[test]
    fn test_format_multiple_placeholders() {
        let args = vec![
            json!("{} {} {}"),
            json!("This is a test"),
            json!("with"),
            json!("multiple placeholders!"),
        ];
        let result = format(args).unwrap();
        assert_eq!(result, json!("This is a test with multiple placeholders!"));
    }

    #[test]
    fn test_format_number() {
        let args = vec![json!("This is a number: {}"), json!(42)];
        let result = format(args).unwrap();
        assert_eq!(result, json!("This is a number: 42"));
    }

    #[test]
    fn test_format_boolean() {
        let args = vec![json!("This is a boolean: {}"), json!(true)];
        let result = format(args).unwrap();
        assert_eq!(result, json!("This is a boolean: true"));
    }

    #[test]
    fn test_format_null() {
        let args = vec![json!("This is {}"), json!(Value::Null)];
        let result = format(args).unwrap();
        assert_eq!(result, json!("This is null"));
    }

    #[test]
    fn test_fails_with_expected_error_for_invalid_argument() {
        let args = vec![json!(42)];
        let result = format(args);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, FunctionCallError::InvalidArgument(_)));
        assert_eq!(
            err.to_string(),
            "function call error: invalid argument: format function requires the first argument to be a string"
        );
    }

    #[test]
    fn test_fails_with_expected_error_for_incorrect_number_of_arguments() {
        let args = vec![];
        let result = format(args);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(
            err,
            FunctionCallError::IncorrectNumberOfArguments(_)
        ));
        assert_eq!(
            err.to_string(),
            "function call error: incorrect number of arguments: format function requires at least one argument"
        );
    }

    #[test]
    fn test_fails_when_format_argument_is_an_array() {
        let args = vec![json!("Format {}"), json!(["This is an array"])];
        let result = format(args);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, FunctionCallError::InvalidArgument(_)));
        assert_eq!(
            err.to_string(),
            "function call error: invalid argument: format function does not support arrays or objects as arguments"
        );
    }

    #[test]
    fn test_fails_when_format_argument_is_an_object() {
        let args = vec![json!("Format {}"), json!({"key": "value"})];
        let result = format(args);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, FunctionCallError::InvalidArgument(_)));
        assert_eq!(
            err.to_string(),
            "function call error: invalid argument: format function does not support arrays or objects as arguments"
        );
    }

    #[test]
    fn test_fails_when_incorrect_number_of_arguments_follow_format_string() {
        let args = vec![json!("Format {} {}")];
        let result = format(args);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(
            err,
            FunctionCallError::IncorrectNumberOfArguments(_)
        ));
        assert_eq!(
            err.to_string(),
            "function call error: incorrect number of arguments: format function requires \
            2 arguments after the format string, one for each \"{}\" placeholder"
        );
    }
}
