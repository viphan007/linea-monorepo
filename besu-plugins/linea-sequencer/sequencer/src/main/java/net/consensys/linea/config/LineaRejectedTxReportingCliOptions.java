/*
 * Copyright Consensys Software Inc.
 *
 * This file is dual-licensed under either the MIT license or Apache License 2.0.
 * See the LICENSE-MIT and LICENSE-APACHE files in the repository root for details.
 *
 * SPDX-License-Identifier: MIT OR Apache-2.0
 */
package net.consensys.linea.config;

import com.google.common.base.MoreObjects;
import java.net.URL;
import net.consensys.linea.plugins.LineaCliOptions;
import picocli.CommandLine.Option;

/** The Linea Rejected Transaction Reporting CLI options. */
public class LineaRejectedTxReportingCliOptions implements LineaCliOptions {
  /**
   * The configuration key used in AbstractLineaPrivateOptionsPlugin to identify the cli options.
   */
  public static final String CONFIG_KEY = "rejected-tx-reporting-config";

  /** The rejected transaction endpoint. */
  public static final String REJECTED_TX_ENDPOINT = "--plugin-linea-rejected-tx-endpoint";

  /** The Linea node type. */
  public static final String LINEA_NODE_TYPE = "--plugin-linea-node-type";

  @Option(
      names = {REJECTED_TX_ENDPOINT},
      hidden = true,
      paramLabel = "<URL>",
      description =
          "Endpoint URI for reporting rejected transactions. Specify a valid URI to enable reporting.")
  URL rejectedTxEndpoint = null;

  @Option(
      names = {LINEA_NODE_TYPE},
      hidden = true,
      paramLabel = "<NODE_TYPE>",
      description =
          "Linea Node type to use when reporting rejected transactions. (Valid values: ${COMPLETION-CANDIDATES})")
  LineaNodeType lineaNodeType = null;

  /** Default constructor. */
  private LineaRejectedTxReportingCliOptions() {}

  /**
   * Create Linea Rejected Transaction Reporting CLI options.
   *
   * @return the Linea Rejected Transaction Reporting CLI options
   */
  public static LineaRejectedTxReportingCliOptions create() {
    return new LineaRejectedTxReportingCliOptions();
  }

  /**
   * Instantiates a new Linea rejected tx reporting cli options from Configuration object
   *
   * @param config An instance of LineaRejectedTxReportingConfiguration
   */
  public static LineaRejectedTxReportingCliOptions fromConfig(
      final LineaRejectedTxReportingConfiguration config) {
    final LineaRejectedTxReportingCliOptions options = create();
    options.rejectedTxEndpoint = config.rejectedTxEndpoint();
    options.lineaNodeType = config.lineaNodeType();
    return options;
  }

  @Override
  public LineaRejectedTxReportingConfiguration toDomainObject() {
    // perform validation here, if endpoint is specified then node type is required.
    // We can ignore node type if endpoint is not specified.
    if (rejectedTxEndpoint != null && lineaNodeType == null) {
      throw new IllegalArgumentException(
          "Error: Missing required argument(s): " + LINEA_NODE_TYPE + "=<NODE_TYPE>");
    }

    return LineaRejectedTxReportingConfiguration.builder()
        .rejectedTxEndpoint(rejectedTxEndpoint)
        .lineaNodeType(lineaNodeType)
        .build();
  }

  @Override
  public String toString() {

    return MoreObjects.toStringHelper(this)
        .add(REJECTED_TX_ENDPOINT, rejectedTxEndpoint)
        .add(LINEA_NODE_TYPE, lineaNodeType)
        .toString();
  }
}
